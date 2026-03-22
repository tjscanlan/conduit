package storage

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
)

const SpanSubject = "spans.ingest"
const streamName = "SPANS"

// NATSPublisher publishes serialized span bytes to a NATS JetStream subject.
type NATSPublisher struct {
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
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     streamName,
		Subjects: []string{SpanSubject},
	})
	if err != nil {
		if _, infoErr := js.StreamInfo(streamName); infoErr != nil {
			return nil, fmt.Errorf("create stream: %w", err)
		}
	}

	return &NATSPublisher{js: js}, nil
}

// Publish sends pre-serialized (proto-marshaled) span bytes to the ingest subject.
func (p *NATSPublisher) Publish(_ context.Context, data []byte) error {
	_, err := p.js.Publish(SpanSubject, data)
	return err
}
