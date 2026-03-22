package telemetry

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/nats-io/nats.go"
)

const spanSubject = "spans.ingest"
const consumerName = "collector"

// Collector subscribes to NATS JetStream and fans out validated spans to the Exporter.
type Collector struct {
	js       nats.JetStreamContext
	exporter *Exporter
	logger   *slog.Logger
}

func NewCollector(natsURL string, exporter *Exporter, logger *slog.Logger) (*Collector, error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, err
	}
	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}
	return &Collector{js: js, exporter: exporter, logger: logger}, nil
}

// Start subscribes and blocks until ctx is cancelled.
func (c *Collector) Start(ctx context.Context) error {
	sub, err := c.js.Subscribe(spanSubject, func(msg *nats.Msg) {
		var in SpanInput
		if err := json.Unmarshal(msg.Data, &in); err != nil {
			c.logger.Error("unmarshal span", "err", err)
			_ = msg.Nak()
			return
		}

		req, err := Process(in)
		if err != nil {
			c.logger.Warn("invalid span — acking to avoid redelivery loop", "err", err)
			_ = msg.Ack()
			return
		}

		if err := c.exporter.Export(ctx, req); err != nil {
			c.logger.Error("export span", "err", err)
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
	}, nats.Durable(consumerName), nats.AckExplicit())
	if err != nil {
		return err
	}
	defer sub.Unsubscribe() //nolint:errcheck

	<-ctx.Done()
	return nil
}
