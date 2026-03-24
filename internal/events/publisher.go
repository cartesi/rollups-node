package events

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/cartesi/rollups-node/pkg/service"
)

type publisherService struct {
	// Logger is a structured logger.
	Logger *slog.Logger

	// Name is a name of the service. It should generally be used to prefix all
	// log lines the service emits.
	Name string

	driver PublisherDriver
}

func NewPublisher(driver PublisherDriver) Publisher {
	return &publisherService{
		// TODO: review this values. Should come from a config.
		Logger: service.NewLogger(slog.LevelDebug, true),
		Name:   "Publisher",
		driver: driver,
	}
}

func (s *publisherService) Publish(ctx context.Context, event Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	topic := string(event.Type)
	s.Logger.DebugContext(ctx, s.Name+": Notify event", "topic", topic)
	return s.driver.Notify(ctx, &Notification{
		Topic: string(event.Type),
		Payload: string(payload),
	})
}
