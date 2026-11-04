package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

const SpanSubject = "spans.ingest"
const streamName = "SPANS"

// NATSPublisher publishes serialized span bytes to a NATS JetStream subject.
type NATSPublisher struct {
	nc *nats.Conn
	js nats.JetStreamContext
}

func NewNATSPublisher(url string) (*NATSPublisher, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("jetstream context: %w", err)
	}

	for _, cfg := range []*nats.StreamConfig{spanStreamConfig(), logStreamConfig()} {
		if err := ensureStream(js, cfg); err != nil {
			return nil, err
		}
	}

	return &NATSPublisher{nc: nc, js: js}, nil
}

// spanStreamConfig is a work queue: a span is deleted once the collector acks
// it. MaxAge bounds disk use if the collector is down for a long time. The
// default (limits) retention kept every span forever, even after it was acked.
//
// JetStream can't change an existing stream's retention policy, so a SPANS
// stream created by an older version keeps limits retention until it is
// deleted and recreated: `nats stream rm SPANS`.
func spanStreamConfig() *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{SpanSubject},
		Retention: nats.WorkQueuePolicy,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   nats.FileStorage,
	}
}

// Ready reports whether the NATS connection is currently up.
func (p *NATSPublisher) Ready(context.Context) error {
	return NATSReady(p.nc)
}

// NATSReady reports whether nc is currently connected. The collector's
// subscriber uses it too.
func NATSReady(nc *nats.Conn) error {
	if nc == nil || !nc.IsConnected() {
		status := "nil"
		if nc != nil {
			status = nc.Status().String()
		}
		return fmt.Errorf("nats not connected (%s)", status)
	}
	return nil
}

// LogSubject and logStreamName carry log records, in their own work queue so a
// backlog of one kind never blocks the other.
const (
	LogSubject    = "logs.ingest"
	logStreamName = "LOGS"
)

func logStreamConfig() *nats.StreamConfig {
	cfg := spanStreamConfig()
	cfg.Name, cfg.Subjects = logStreamName, []string{LogSubject}
	return cfg
}

// ensureStream creates cfg's stream unless it already exists.
func ensureStream(js nats.JetStreamContext, cfg *nats.StreamConfig) error {
	if _, err := js.AddStream(cfg); err != nil {
		if _, infoErr := js.StreamInfo(cfg.Name); infoErr != nil {
			return fmt.Errorf("create stream %s: %w", cfg.Name, err)
		}
	}
	return nil
}

// PublishLog sends a serialized log record (JSON-encoded telemetry.LogInput).
func (p *NATSPublisher) PublishLog(_ context.Context, data []byte) error {
	_, err := p.js.Publish(LogSubject, data)
	return err
}

// Publish sends a serialized span (JSON-encoded telemetry.SpanInput) to the ingest subject.
func (p *NATSPublisher) Publish(_ context.Context, data []byte) error {
	_, err := p.js.Publish(SpanSubject, data)
	return err
}
