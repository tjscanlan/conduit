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

	// Ensure stream exists; ignore error if it already does.
	_, err = js.AddStream(spanStreamConfig())
	if err != nil {
		if _, infoErr := js.StreamInfo(streamName); infoErr != nil {
			return nil, fmt.Errorf("create stream: %w", err)
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

// Publish sends a serialized span (JSON-encoded telemetry.SpanInput) to the ingest subject.
func (p *NATSPublisher) Publish(_ context.Context, data []byte) error {
	_, err := p.js.Publish(SpanSubject, data)
	return err
}
