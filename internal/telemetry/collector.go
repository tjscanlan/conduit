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

const consumerName = "collector"

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
	logger   *slog.Logger
}

// disposition is what to do with a message after handling it.
type disposition int

const (
	ack   disposition = iota // processed, or permanently invalid
	retry                    // transient failure: redeliver after retryDelay
	term                     // undecodable: never redeliver
)

func NewCollector(natsURL string, exporter *Exporter, logger *slog.Logger) (*Collector, error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, err
	}
	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}
	return &Collector{nc: nc, js: js, exporter: exporter, logger: logger}, nil
}

// Ready reports whether the collector's NATS connection is up.
func (c *Collector) Ready(context.Context) error {
	return storage.NATSReady(c.nc)
}

// Start subscribes and blocks until ctx is cancelled.
func (c *Collector) Start(ctx context.Context) error {
	sub, err := c.js.Subscribe(storage.SpanSubject, func(msg *nats.Msg) {
		switch c.handle(ctx, msg.Data) {
		case ack:
			_ = msg.Ack()
		case retry:
			_ = msg.NakWithDelay(retryDelay)
		case term:
			_ = msg.Term()
		}
	}, nats.Durable(consumerName), nats.AckExplicit())
	if err != nil {
		return err
	}
	defer sub.Unsubscribe() //nolint:errcheck

	<-ctx.Done()
	return nil
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
