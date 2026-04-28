// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"fmt"
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
	wsClient             *MockEthClient
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
	s.ctx, s.cancel = context.WithTimeout(context.Background(), suiteTimeout)
	config.SetDefaults()
}

func (s *EvmReaderSuite) TearDownSuite() {
	s.cancel()
}

func (s *EvmReaderSuite) SetupTest() {
	s.client = newMockEthClient().SetupDefaultBehavior()
	s.wsClient = newMockEthClient().SetupDefaultWsBehavior()
	s.repository = newMockRepository().SetupDefaultBehavior()
	s.applicationContract1 = newMockApplicationContract().SetupDefaultBehavior()
	s.applicationContract2 = newMockApplicationContract().SetupDefaultBehavior()
	s.inputBox = newMockInputBox().SetupDefaultBehavior()
	s.contractFactory = newMockAdapterFactory().SetupDefaultBehavior(s.applicationContract1, s.applicationContract2, s.inputBox)

	s.evmReader = &Service{
		client:                              s.client,
		wsClient:                            s.wsClient,
		repository:                          s.repository,
		defaultBlock:                        DefaultBlock_Latest,
		adapterFactory:                      s.contractFactory,
		hasEnabledApps:                      true,
		inputReaderEnabled:                  true,
		blockchainMaxRetries:                0,
		blockchainSubscriptionRetryInterval: time.Second,
		wsLivenessTimeout:                   120 * time.Second,
	}

	logLevel, err := config.GetLogLevel()
	s.Require().NoError(err)

	serviceArgs := &service.ServiceConfigs{Name: "evm-reader", LogLevel: logLevel}
	err = service.InitServiceTemplate(serviceArgs, &s.evmReader.ServiceTemplate, s.evmReader)
	s.Require().NoError(err)
}

// Service tests
func (s *EvmReaderSuite) TestItStopsWhenContextIsCanceled() {
	ctx, cancel := context.WithCancel(s.ctx)
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)
	go func() {
		errChannel <- s.evmReader.Run(ctx, ready)
	}()
	cancel()

	err := <-errChannel
	s.Require().Nil(err, "stopped with an error when canceled")
}

func (s *EvmReaderSuite) TestItEventuallyBecomesReady() {
	ready := make(chan struct{}, 1)
	errChannel := make(chan error, 1)
	go func() {
		errChannel <- s.evmReader.Run(s.ctx, ready)
	}()

	select {
	case <-ready:
	case err := <-errChannel:
		s.FailNow("unexpected failure", err)
	}
}

func (s *EvmReaderSuite) TestItReturnsErrorWhenWebSocketStalls() {
	s.evmReader.wsLivenessTimeout = 50 * time.Millisecond
	ready := make(chan struct{}, 1)
	headersProcessed, err := s.evmReader.watchForNewBlocks(s.ctx, ready)
	s.Require().Equal(uint64(0), headersProcessed)
	var subErr *SubscriptionError
	s.Require().ErrorAs(err, &subErr)
	s.Require().ErrorContains(err, "no new block header received")
}

func (s *EvmReaderSuite) TestRunExhaustsRetriesOnConsecutiveConnectionFailures() {
	s.evmReader.blockchainMaxRetries = 2
	s.evmReader.blockchainSubscriptionRetryInterval = time.Millisecond

	s.wsClient.Unset("SubscribeNewHead")
	sub := &MockSubscription{}
	s.wsClient.On("SubscribeNewHead", mock.Anything, mock.Anything).
		Return(sub, fmt.Errorf("connection refused"))

	err := s.evmReader.Run(s.ctx, make(chan struct{}, 1))
	s.Require().ErrorContains(err, "connection refused")
	// 1 initial + 2 retries = 3 calls
	s.wsClient.AssertNumberOfCalls(s.T(), "SubscribeNewHead", 3)
}

func (s *EvmReaderSuite) TestRunResetsRetriesAfterProcessingHeaders() {
	s.evmReader.blockchainMaxRetries = 1
	s.evmReader.blockchainSubscriptionRetryInterval = time.Millisecond
	s.evmReader.wsLivenessTimeout = 100 * time.Millisecond

	// First call: subscribe succeeds, deliver a header, then subscription error fires.
	// -> headersProcessed > 0, so consecutiveFailures resets to 0
	// Second call: subscribe fails (connection error) -> consecutiveFailures=1
	// Third call: subscribe fails -> consecutiveFailures=2 > maxRetries(1) -> exit
	subWithError := &MockSubscription{}
	errCh := make(chan error, 1)
	subWithError.On("Unsubscribe").Return()
	subWithError.On("Err").Return((<-chan error)(errCh))

	s.wsClient.Unset("SubscribeNewHead")
	s.wsClient.On("SubscribeNewHead", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			ch := args.Get(1).(chan<- *types.Header)
			// Deliver a header then trigger subscription error
			go func() {
				ch <- &header0
				errCh <- fmt.Errorf("connection lost")
			}()
		}).
		Return(subWithError, nil).Once()

	emptySub := &MockSubscription{}
	s.wsClient.On("SubscribeNewHead", mock.Anything, mock.Anything).
		Return(emptySub, fmt.Errorf("connection refused"))

	err := s.evmReader.Run(s.ctx, make(chan struct{}, 1))
	s.Require().ErrorContains(err, "connection refused")
	// 1 successful + 1 retry + 1 exhausted = 3 calls
	s.wsClient.AssertNumberOfCalls(s.T(), "SubscribeNewHead", 3)
}

func (s *EvmReaderSuite) TestRunDoesNotResetRetriesWithoutProcessingHeaders() {
	s.evmReader.blockchainMaxRetries = 1
	s.evmReader.blockchainSubscriptionRetryInterval = time.Millisecond
	s.evmReader.wsLivenessTimeout = time.Millisecond

	// Subscribe succeeds but no headers arrive before liveness timeout.
	// headersProcessed=0, so consecutiveFailures increments (not reset).
	// With maxRetries=1: first timeout -> failures=1, second timeout -> failures=2 > 1 -> exit
	err := s.evmReader.Run(s.ctx, make(chan struct{}, 1))
	s.Require().ErrorContains(err, "no new block header received")
	s.wsClient.AssertNumberOfCalls(s.T(), "SubscribeNewHead", 2)
}

func (s *EvmReaderSuite) TestRunStopsDuringRetryWhenContextCanceled() {
	s.evmReader.blockchainMaxRetries = 100
	s.evmReader.blockchainSubscriptionRetryInterval = time.Second

	s.wsClient.Unset("SubscribeNewHead")
	sub := &MockSubscription{}
	ctx, cancel := context.WithCancel(s.ctx)
	s.wsClient.On("SubscribeNewHead", mock.Anything, mock.Anything).
		Run(func(_ mock.Arguments) { cancel() }).
		Return(sub, fmt.Errorf("connection refused"))

	err := s.evmReader.Run(ctx, make(chan struct{}, 1))
	s.Require().Nil(err)
}

func (s *EvmReaderSuite) TestItFailsToSubscribeForNewInputsOnStart() {
	s.wsClient.Unset("ChainID")
	s.wsClient.Unset("SubscribeNewHead")
	emptySubscription := &MockSubscription{}
	s.wsClient.On(
		"SubscribeNewHead",
		mock.Anything,
		mock.Anything,
	).Return(emptySubscription, fmt.Errorf("expected failure"))

	err := s.evmReader.Run(s.ctx, make(chan struct{}, 1))
	s.Require().ErrorContains(err, "expected failure")
	s.wsClient.AssertNumberOfCalls(s.T(), "SubscribeNewHead", 1)
	s.wsClient.AssertExpectations(s.T())
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
