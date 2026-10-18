// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

type observationIterator struct {
	remaining int
	err       error
	closeErr  error
	closed    bool
}

func TestLatchedClaimFinalityFailureBlocksLaterHealthyPublication(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	queueDelivery(t, backend, lookup, 1)
	cause := errors.Join(txsender.ErrFinality, errors.New("canonical finalized hash changed"))
	s.failure.Store(&cause)
	require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrFinality)
	require.Empty(t, backend.published)
	require.Empty(t, backend.callRefs)
	require.Empty(t, s.publisher.cooldowns)
}

func TestProtectedRPCRecoveryNeedsNoCapabilityStateMachine(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, _ := queueDelivery(t, backend, lookup, 1)
	refusal := errors.New("application RPC temporarily unavailable")
	backend.fail[app.IApplicationAddress] = refusal
	require.ErrorIs(t, s.publisher.tick(t.Context()), refusal)
	require.Nil(t, s.failure.Load())
	delete(backend.fail, app.IApplicationAddress)
	clear(s.publisher.cooldowns)
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 1)
}

func TestHardObservationFailureBlocksTheIndependentPublisher(t *testing.T) {
	for _, cause := range []error{txsender.ErrFinality, txsender.ErrContent} {
		t.Run(cause.Error(), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			queueDelivery(t, backend, lookup, 1)
			chain := s.blockchain.(*claimerBlockchainMock)
			chain.On("getDefaultBlockNumber", mock.Anything).Return((*big.Int)(nil), cause).Once()
			_, err := s.Tick(t.Context())
			require.ErrorIs(t, err, cause)
			require.NotNil(t, s.failure.Load())
			require.ErrorIs(t, s.publisher.tick(t.Context()), cause)
			require.Empty(t, backend.published)
			chain.AssertExpectations(t)
		})
	}
}

func (it *observationIterator) Next() bool {
	if it.remaining == 0 {
		return false
	}
	it.remaining--
	return true
}
func (it *observationIterator) Close() error {
	it.closed = true
	return it.closeErr
}
func (it *observationIterator) Error() error { return it.err }

func TestObservedEventWindowRetainsIteratorCloseFailures(t *testing.T) {
	closeErr := errors.New("event iterator close failed")
	callbackErr := errors.New("observed log is invalid")
	iteratorErr := errors.New("event subscription failed")
	for _, failure := range []struct {
		name     string
		callback error
		iterator error
	}{
		{"complete window", nil, nil},
		{"callback failure", callbackErr, nil},
		{"iterator failure", nil, iteratorErr},
	} {
		t.Run(failure.name, func(t *testing.T) {
			it := &observationIterator{remaining: 1, err: failure.iterator, closeErr: closeErr}
			onHit := newOnHit(t.Context(), common.HexToAddress("0x1"),
				func(*bind.FilterOpts, []common.Address, []common.Address) (*observationIterator, error) {
					return it, nil
				},
				func(_ context.Context, block uint64) (*big.Int, error) { return new(big.Int).SetUint64(block), nil },
				func(*observationIterator) error { return failure.callback })
			err := onHit(50)
			require.ErrorIs(t, err, closeErr)
			if failure.callback != nil {
				require.ErrorIs(t, err, failure.callback)
			}
			if failure.iterator != nil {
				require.ErrorIs(t, err, failure.iterator)
			}
			require.True(t, it.closed)
		})
	}
}

func TestCounterWindowRejectsIncompleteLogs(t *testing.T) {
	for _, count := range []int{0, 1, 2, 3} {
		t.Run(big.NewInt(int64(count)).String(), func(t *testing.T) {
			onHit := newOnHit(t.Context(), common.HexToAddress("0x1"),
				func(opts *bind.FilterOpts, _, addresses []common.Address) (*observationIterator, error) {
					require.Equal(t, uint64(50), opts.Start)
					require.Equal(t, uint64(50), *opts.End)
					require.Equal(t, []common.Address{common.HexToAddress("0x1")}, addresses)
					return &observationIterator{remaining: count}, nil
				}, func(_ context.Context, block uint64) (*big.Int, error) {
					if block == 49 {
						return big.NewInt(7), nil
					}
					require.Equal(t, uint64(50), block)
					return big.NewInt(9), nil
				}, func(*observationIterator) error { return nil })
			if count == 2 {
				require.NoError(t, onHit(50))
			} else {
				require.ErrorIs(t, onHit(50), txsender.ErrInconclusive)
			}
		})
	}
}
