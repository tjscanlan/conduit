package telemetry

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/your-org/agent-observability/internal/schema"
	"github.com/your-org/agent-observability/internal/storage"
)

const (
	consumerName       = "collector"
	logConsumerName    = "collector-logs"
	metricConsumerName = "collector-metrics"
)

// metricSink is where validated metric samples go (VictoriaMetrics). Unlike
// span-duration export, it is the system of record for metrics: failures retry.
type metricSink interface {
	WriteMetrics(ctx context.Context, metrics []storage.VMMetric) error
}

// logStore is where validated log records go (ClickHouse).
type logStore interface {
	InsertLog(ctx context.Context, row storage.LogRow) error
}

// retryDelay spaces out redeliveries while a backend is down, instead of
// JetStream redelivering immediately in a hot loop.
const retryDelay = 5 * time.Second

type spanExporter interface {
	Export(ctx context.Context, req *schema.SpanRequest) error
}

// Collector subscribes to NATS JetStream and fans out validated spans to the Exporter.
type Collector struct {
	nc       *nats.Conn
	js       nats.JetStreamContext
	exporter spanExporter
	logs     logStore
	metrics  metricSink
	logger   *slog.Logger
}

// disposition is what to do with a message after handling it.
type disposition int

const (
	ack   disposition = iota // processed, or permanently invalid
	retry                    // transient failure: redeliver after retryDelay
	term                     // undecodable: never redeliver
)

func NewCollector(natsURL string, exporter *Exporter, logs logStore, metrics metricSink, logger *slog.Logger) (*Collector, error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, err
	}
	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}
	return &Collector{nc: nc, js: js, exporter: exporter, logs: logs, metrics: metrics, logger: logger}, nil
}

// Ready reports whether the collector's NATS connection is up.
func (c *Collector) Ready(context.Context) error {
	return storage.NATSReady(c.nc)
}

// Start subscribes to spans and logs and blocks until ctx is cancelled.
func (c *Collector) Start(ctx context.Context) error {
	dispose := func(handle func(context.Context, []byte) disposition) nats.MsgHandler {
		return func(msg *nats.Msg) {
			switch handle(ctx, msg.Data) {
			case ack:
				_ = msg.Ack()
			case retry:
				_ = msg.NakWithDelay(retryDelay)
			case term:
				_ = msg.Term()
			}
		}
	}
	spans, err := c.js.Subscribe(storage.SpanSubject, dispose(c.handle), nats.Durable(consumerName), nats.AckExplicit())
	if err != nil {
		return err
	}
	defer spans.Unsubscribe() //nolint:errcheck
	logs, err := c.js.Subscribe(storage.LogSubject, dispose(c.handleLog), nats.Durable(logConsumerName), nats.AckExplicit())
	if err != nil {
		return err
	}
	defer logs.Unsubscribe() //nolint:errcheck
	metrics, err := c.js.Subscribe(storage.MetricSubject, dispose(c.handleMetric), nats.Durable(metricConsumerName), nats.AckExplicit())
	if err != nil {
		return err
	}
	defer metrics.Unsubscribe() //nolint:errcheck

	<-ctx.Done()
	return nil
}

// handleMetric writes one metric sample: same delivery rules as spans and logs.
func (c *Collector) handleMetric(ctx context.Context, data []byte) disposition {
	var in MetricInput
	if err := json.Unmarshal(data, &in); err != nil {
		c.logger.Error("unmarshal metric: terminating message", "err", err)
		return term
	}
	req, err := ProcessMetric(in, time.Now())
	if err != nil {
		c.logger.Warn("invalid metric: acking to avoid redelivery loop", "err", err)
		return ack
	}
	if err := c.metrics.WriteMetrics(ctx, []storage.VMMetric{{
		Name: req.Name, Labels: req.Labels, Value: req.Value, Timestamp: req.Timestamp,
	}}); err != nil {
		c.logger.Error("write metric: will retry", "err", err)
		return retry
	}
	return ack
}

// handleLog stores one log record: same delivery rules as spans.
func (c *Collector) handleLog(ctx context.Context, data []byte) disposition {
	var in LogInput
	if err := json.Unmarshal(data, &in); err != nil {
		c.logger.Error("unmarshal log: terminating message", "err", err)
		return term
	}
	req, err := ProcessLog(in, time.Now())
	if err != nil {
		c.logger.Warn("invalid log: acking to avoid redelivery loop", "err", err)
		return ack
	}
	if err := c.logs.InsertLog(ctx, storage.LogRow{
		Timestamp: req.Timestamp, AgentName: req.AgentName, Severity: req.Severity, Body: req.Body,
		Attributes: req.Attributes, TraceID: req.TraceID, SpanID: req.SpanID,
	}); err != nil {
		c.logger.Error("insert log: will retry", "err", err)
		return retry
	}
	return ack
}

func (c *Collector) handle(ctx context.Context, data []byte) disposition {
	var in SpanInput
	if err := json.Unmarshal(data, &in); err != nil {
		// Redelivering bytes that can't be decoded would loop forever.
		c.logger.Error("unmarshal span: terminating message", "err", err)
		return term
	}

	req, err := Process(in)
	if err != nil {
		c.logger.Warn("invalid span: acking to avoid redelivery loop", "err", err)
		return ack
	}

	if err := c.exporter.Export(ctx, req); err != nil {
		c.logger.Error("export span: will retry", "err", err, "trace_id", req.TraceID, "span_id", req.SpanID)
		return retry
	}
	return ack
}
