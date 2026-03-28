package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"cloud.google.com/go/pubsub"

	"github.com/joaovitorpedrosomonteiro/low-carbon-notification-service/internal/application"
)

type EventHandler interface {
	HandleInventoryStateChanged(ctx context.Context, envelope application.EventEnvelope) error
	HandleAuditorAccessGranted(ctx context.Context, envelope application.EventEnvelope) error
	HandleDocumentReadyForSigning(ctx context.Context, envelope application.EventEnvelope) error
	HandleDocumentGenerated(ctx context.Context, envelope application.EventEnvelope) error
	HandleDocumentGenerationFailed(ctx context.Context, envelope application.EventEnvelope) error
	HandleUserCreated(ctx context.Context, envelope application.EventEnvelope) error
	HandleUserPasswordReset(ctx context.Context, envelope application.EventEnvelope) error
	HandlePasswordResetRequested(ctx context.Context, envelope application.EventEnvelope) error
}

type Deduplicator interface {
	IsDuplicate(ctx context.Context, eventID string) (bool, error)
}

type RedisDeduplicator struct {
	client interface {
		SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
	}
}

func NewRedisDeduplicator(client interface {
	SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
}) *RedisDeduplicator {
	return &RedisDeduplicator{client: client}
}

func (d *RedisDeduplicator) IsDuplicate(ctx context.Context, eventID string) (bool, error) {
	key := fmt.Sprintf("notif:dedup:%s", eventID)
	ok, err := d.client.SetNX(ctx, key, "1", 24*time.Hour)
	if err != nil {
		return false, err
	}
	return !ok, nil
}

type MockDeduplicator struct{}

func NewMockDeduplicator() *MockDeduplicator {
	return &MockDeduplicator{}
}

func (d *MockDeduplicator) IsDuplicate(ctx context.Context, eventID string) (bool, error) {
	return false, nil
}

type Subscriber struct {
	client       *pubsub.Client
	handlers     EventHandler
	deduplicator Deduplicator
}

func NewSubscriber(ctx context.Context, handlers EventHandler, deduplicator Deduplicator) (*Subscriber, error) {
	projectID := os.Getenv("GCP_PROJECT_ID")
	if projectID == "" {
		projectID = "low-carbon-491109"
	}

	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub.NewClient: %w", err)
	}

	return &Subscriber{
		client:       client,
		handlers:     handlers,
		deduplicator: deduplicator,
	}, nil
}

func (s *Subscriber) Start(ctx context.Context) error {
	subscriptions := map[string]string{
		"inventory-events": os.Getenv("INVENTORY_SUBSCRIPTION"),
		"identity-events":  os.Getenv("IDENTITY_SUBSCRIPTION"),
		"document-events":  os.Getenv("DOCUMENT_SUBSCRIPTION"),
	}

	for topic, subID := range subscriptions {
		if subID == "" {
			subID = fmt.Sprintf("notification-service-%s", topic)
		}

		sub := s.client.Subscription(subID)
		go s.receiveLoop(ctx, topic, sub)
		log.Printf("[Subscriber] Listening on subscription: %s (topic: %s)", subID, topic)
	}

	return nil
}

func (s *Subscriber) receiveLoop(ctx context.Context, topic string, sub *pubsub.Subscription) {
	err := sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		if err := s.processMessage(ctx, msg); err != nil {
			log.Printf("[Subscriber] Error processing message from %s: %v", topic, err)
			msg.Nack()
			return
		}
		msg.Ack()
	})
	if err != nil && ctx.Err() == nil {
		log.Printf("[Subscriber] Receive loop error for %s: %v", topic, err)
	}
}

func (s *Subscriber) processMessage(ctx context.Context, msg *pubsub.Message) error {
	var envelope application.EventEnvelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}

	isDup, err := s.deduplicator.IsDuplicate(ctx, envelope.EventID)
	if err != nil {
		log.Printf("[Subscriber] Dedup check failed: %v", err)
	}
	if isDup {
		log.Printf("[Subscriber] Duplicate event %s, skipping", envelope.EventID)
		return nil
	}

	log.Printf("[Subscriber] Processing event %s (type: %s)", envelope.EventID, envelope.EventType)

	switch envelope.EventType {
	case "InventoryStateChanged":
		return s.handlers.HandleInventoryStateChanged(ctx, envelope)
	case "AuditorAccessGranted":
		return s.handlers.HandleAuditorAccessGranted(ctx, envelope)
	case "DocumentReadyForSigning":
		return s.handlers.HandleDocumentReadyForSigning(ctx, envelope)
	case "DocumentGenerated":
		return s.handlers.HandleDocumentGenerated(ctx, envelope)
	case "DocumentGenerationFailed":
		return s.handlers.HandleDocumentGenerationFailed(ctx, envelope)
	case "UserCreated":
		return s.handlers.HandleUserCreated(ctx, envelope)
	case "UserPasswordReset":
		return s.handlers.HandleUserPasswordReset(ctx, envelope)
	case "PasswordResetRequested":
		return s.handlers.HandlePasswordResetRequested(ctx, envelope)
	default:
		log.Printf("[Subscriber] Unknown event type: %s", envelope.EventType)
		return nil
	}
}

func (s *Subscriber) Close() error {
	return s.client.Close()
}
