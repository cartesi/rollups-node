// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const (
	suiteTimeout = 120 * time.Second
)

type EvmReaderSuite struct {
	suite.Suite
	ctx                  context.Context
	cancel               context.CancelFunc
	client               *MockEthClient
	repository           *MockRepository
	evmReader            *Service
	contractFactory      *MockAdapterFactory
	applicationContract1 *MockApplicationContract
	applicationContract2 *MockApplicationContract
	inputBox             *MockInputBox
}

func TestEvmReaderSuite(t *testing.T) {
	suite.Run(t, new(EvmReaderSuite))
}

func (s *EvmReaderSuite) SetupSuite() {
	config.SetDefaults()
}

func (s *EvmReaderSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), suiteTimeout)
	s.client = newMockEthClient().SetupDefaultBehavior()
	s.repository = newMockRepository().SetupDefaultBehavior()
	s.applicationContract1 = newMockApplicationContract().SetupDefaultBehavior()
	s.applicationContract2 = newMockApplicationContract().SetupDefaultBehavior()
	s.inputBox = newMockInputBox().SetupDefaultBehavior()
	s.contractFactory = newMockAdapterFactory().SetupDefaultBehavior(s.applicationContract1, s.applicationContract2, s.inputBox)

	s.evmReader = &Service{
		client:             s.client,
		repository:         s.repository,
		defaultBlock:       DefaultBlock_Latest,
		inputReaderEnabled: true,
		hasEnabledApps:     true,
		adapterFactory:     s.contractFactory,
	}

	logLevel, err := config.GetLogLevel()
	s.Require().NoError(err)

	serviceArgs := &service.CreateInfo{
		Name:         "evm-reader",
		Impl:         s.evmReader,
		LogLevel:     logLevel,
		Context:      s.ctx,
		Cancel:       s.cancel,
		PollInterval: 100 * time.Millisecond,
	}
	err = service.Create(context.Background(), serviceArgs, &s.evmReader.Service)
	s.Require().NoError(err)

	s.evmReader.resolver = newApplicationAdapterResolver(s.evmReader.Logger, s.contractFactory)
}

func (s *EvmReaderSuite) TearDownTest() {
	s.cancel()
}

func newCallNotification(c *mock.Call) <-chan struct{} {
	ch := make(chan struct{})
	c.Run(func(mock.Arguments) { ch <- struct{}{} })
	return ch
}

func newBlockedCallNotification(c *mock.Call) (<-chan struct{}, chan struct{}) {
	called := make(chan struct{})
	blocked := make(chan struct{})
	c.Run(func(mock.Arguments) {
		called <- struct{}{} // notify function was called
		<-blocked            // block function until notified
	})
	return called, blocked
}

func waitNotification(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	case <-time.After(2 * time.Second):
		return false
	}
}

func wasntNotified(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return false
	default:
		return true
	}
}

// Service tests
func (s *EvmReaderSuite) TestItStopsWhenContextIsAlreadyCanceled() {
	done := make(chan struct{})
	go func() {
		s.cancel()
		err := s.evmReader.Serve()
		s.Require().NoError(err)
		close(done)
	}()

	s.Require().True(waitNotification(done), "evmreader did not stop after context cancelation")
}

func (s *EvmReaderSuite) TestItStopsWhenContextIsCanceledAfterFirstHeader() {
	called := newCallNotification(s.client.EnqueueNewHead(100))

	done := make(chan struct{})
	go func() {
		err := s.evmReader.Serve()
		s.Require().NoError(err)
		close(done)
	}()

	s.Require().True(waitNotification(called), "evmreader did not read new header")

	s.cancel()

	s.Require().True(waitNotification(done), "evmreader did not stop after context cancelation")
}

func (s *EvmReaderSuite) TestReadyReflectsServeLifecycle() {
	called := newCallNotification(s.client.EnqueueNewHead(100))

	s.Require().False(s.evmReader.Ready())

	done := make(chan struct{})
	go func() {
		err := s.evmReader.Serve()
		s.Require().NoError(err)
		close(done)
	}()

	s.Require().True(waitNotification(called))
	s.Require().True(s.evmReader.Ready())
	s.Require().True(wasntNotified(done))

	s.cancel()
	s.Require().True(waitNotification(done))
	s.Require().False(s.evmReader.Ready())
}

func (s *EvmReaderSuite) TestReadyDoesNotDependOnPollingSuccess() {
	var hdr *types.Header
	called := newCallNotification(s.client.On("HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(hdr, errors.New("transient connection error")).Once())
	s.Require().False(s.evmReader.Ready())

	done := make(chan struct{})
	go func() {
		err := s.evmReader.Serve()
		s.Require().NoError(err)
		close(done)
	}()

	s.Require().True(waitNotification(called))
	s.Require().True(s.evmReader.Ready())
	s.Require().True(wasntNotified(done))

	s.cancel()
	s.Require().True(waitNotification(done))
	s.Require().False(s.evmReader.Ready())
}

func (s *EvmReaderSuite) TestTickScansWithServiceContext() {
	s.client.EnqueueNewHead(100).Once()

	// reset mock calls from 'MockRepository.SetupDefaultBehavior'
	s.repository.On(
		"UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_InputAdded,
		mock.Anything,
	).Unset()
	s.repository.On(
		"UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_OutputExecuted,
		mock.Anything,
	).Unset()

	assertValidContext := func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		s.Require().Equal(s.evmReader.Context, ctx)
		s.Require().Nil(ctx.Err())
	}

	// setup mock calls expected to receive a valid context
	s.repository.On(
		"UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_InputAdded,
		mock.Anything,
	).Return(nil).Times(1).Run(assertValidContext)
	s.repository.On(
		"UpdateEventLastCheckBlock",
		mock.Anything,
		mock.Anything,
		MonitoredEvent_OutputExecuted,
		mock.Anything,
	).Return(nil).Times(4).Run(assertValidContext)

	s.Require().False(s.evmReader.Ready())

	errs := s.evmReader.Tick()
	s.Require().Empty(errs)

	s.client.AssertCalled(s.T(), "HeaderByNumber", mock.Anything, mock.Anything)
	s.repository.AssertNumberOfCalls(s.T(), "UpdateEventLastCheckBlock", 5)
}

func (s *EvmReaderSuite) TestFetchMostRecentHeaderReturnsErrorWhenHeaderNumberIsNil() {
	s.client.On("HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(&types.Header{}, nil).Once()

	blockNumber, err := s.evmReader.fetchMostRecentHeader(s.ctx, DefaultBlock_Latest)

	s.Require().Error(err)
	s.Require().ErrorContains(err, "returned header number is nil")
	s.Require().Zero(blockNumber)
}

func (s *EvmReaderSuite) TestTickReturnsHeaderFetchErrorWithoutLocalErrorLog() {
	var logBuffer bytes.Buffer
	s.evmReader.Logger = slog.New(slog.NewTextHandler(&logBuffer, nil))

	headerErr := errors.New("transient connection error")
	var hdr *types.Header
	s.client.On("HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(hdr, headerErr).Once()

	errs := s.evmReader.Tick()

	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], headerErr)
	s.Require().NotContains(logBuffer.String(), "Error fetching most recent block")
	s.repository.AssertNumberOfCalls(s.T(), "ListApplications", 0)
}

func (s *EvmReaderSuite) TestItRunsWhenConnectionFails() {
	var hdr *types.Header
	called := newCallNotification(s.client.On("HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(hdr, errors.New("transient connection error")))

	done := make(chan struct{})
	go func() {
		err := s.evmReader.Serve()
		s.Require().NoError(err)
		close(done)
	}()

	s.Require().True(waitNotification(called))
	s.Require().True(waitNotification(called))
	s.Require().True(wasntNotified(done))
}

func (s *EvmReaderSuite) TestRunResetsRetriesAfterProcessingHeaders() {
	s.client.EnqueueNewHead(100).Once()
	var hdr *types.Header
	s.client.On("HeaderByNumber",
		mock.Anything,
		mock.Anything,
	).Return(hdr, errors.New("transient connection error")).Once()
	called := newCallNotification(s.client.EnqueueNewHead(101))

	done := make(chan struct{})
	go func() {
		err := s.evmReader.Serve()
		s.Require().NoError(err)
		close(done)
	}()

	s.Require().True(waitNotification(called))
	s.Require().True(wasntNotified(done))

	s.client.AssertCalled(s.T(), "HeaderByNumber", mock.Anything, mock.Anything)
}

// indexApps indexes applications given a key extractor function.
// Only used in tests.
func indexApps[K comparable](
	keyExtractor func(appContracts) K,
	apps []appContracts,
) map[K][]appContracts {
	result := make(map[K][]appContracts)
	for _, item := range apps {
		key := keyExtractor(item)
		result[key] = append(result[key], item)
	}
	return result
}

func (s *EvmReaderSuite) TestIndexApps() {

	s.Run("Ok", func() {
		apps := []appContracts{
			{application: &Application{LastInputCheckBlock: 23}},
			{application: &Application{LastInputCheckBlock: 22}},
			{application: &Application{LastInputCheckBlock: 21}},
			{application: &Application{LastInputCheckBlock: 23}},
		}

		keyByProcessedBlock := func(a appContracts) uint64 {
			return a.application.LastInputCheckBlock
		}

		indexApps := indexApps(keyByProcessedBlock, apps)

		s.Require().Equal(3, len(indexApps))
		apps, ok := indexApps[23]
		s.Require().True(ok)
		s.Require().Equal(2, len(apps))
	})

	s.Run("whenIndexAppsArrayEmpty", func() {
		apps := []appContracts{}

		keyByProcessedBlock := func(a appContracts) uint64 {
			return a.application.LastInputCheckBlock
		}

		indexApps := indexApps(keyByProcessedBlock, apps)

		s.Require().Equal(0, len(indexApps))
	})

	s.Run("whenUsesWrongKey", func() {
		apps := []appContracts{
			{application: &Application{LastInputCheckBlock: 23}},
			{application: &Application{LastInputCheckBlock: 22}},
			{application: &Application{LastInputCheckBlock: 21}},
			{application: &Application{LastInputCheckBlock: 23}},
		}

		keyByProcessedBlock := func(a appContracts) uint64 {
			return a.application.LastInputCheckBlock
		}

		indexApps := indexApps(keyByProcessedBlock, apps)

		s.Require().Equal(3, len(indexApps))
		apps, ok := indexApps[0]
		s.Require().False(ok)
		s.Require().Nil(apps)

	})

}
