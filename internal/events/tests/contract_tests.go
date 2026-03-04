// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contracttests

import (
	"encoding/json"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/cartesi/rollups-node/internal/events"
)

type EventsServiceFactory func() events.Service

type PublisherTestSuite struct {
	suite.Suite
	CreateService EventsServiceFactory
}

func NewPublisherTestSuite(factory EventsServiceFactory) *PublisherTestSuite {
	return &PublisherTestSuite{CreateService: factory}
}

func (s *PublisherTestSuite) TestPublishEvent() {
	req := s.Require()
	ctx := s.T().Context()

	service := s.CreateService()

	s.Run("SingleEvent", func() {
		payload, err := json.Marshal("Hello, World!")
		req.NoError(err)

		expected := events.Event{
			Type:      events.EventAppRegistered,
			AppID:     "echo-dapp",
			Payload:   payload,
			Timestamp: time.Now(),
		}

		go func() {
			ch, err := service.Subscribe(ctx, events.SubscriptionFilter{})
			req.NoError(err)

			actual := <-ch
			req.Equal(actual, expected)
		}()

		err = service.Publish(ctx, expected)
		req.NoError(err)
	})

	// TODO: Multiple subscribers receiving same event

	// TODO: Publishers with multiple events to a single subscriber

	// TODO: Slow subscribers

}
