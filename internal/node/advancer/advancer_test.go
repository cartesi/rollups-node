// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	crand "crypto/rand"
	mrand "math/rand"
	"testing"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/internal/node/nodemachine"

	"github.com/stretchr/testify/suite"
)

func TestAdvancer(t *testing.T) {
	suite.Run(t, new(AdvancerSuite))
}

type AdvancerSuite struct{ suite.Suite }

func (s *AdvancerSuite) TestNew() {
	s.Run("Ok", func() {
		require := s.Require()
		machines := Machines{randomAddress(): &MockMachine{}}
		repository := &MockRepository{}
		advancer, err := New(machines, repository)
		require.NotNil(advancer)
		require.Nil(err)
	})

	s.Run("InvalidMachines", func() {
		require := s.Require()
		repository := &MockRepository{}
		advancer, err := New(nil, repository)
		require.Nil(advancer)
		require.Equal(ErrInvalidMachines, err)
	})

	s.Run("InvalidRepository", func() {
		require := s.Require()
		machines := Machines{randomAddress(): &MockMachine{}}
		advancer, err := New(machines, nil)
		require.Nil(advancer)
		require.Equal(ErrInvalidRepository, err)
	})
}

// NOTE: this test is just the beginning; we need more tests.
func (s *AdvancerSuite) TestRun() {
	require := s.Require()

	appAddress := randomAddress()

	machines := Machines{}
	advanceRes := randomAdvanceResult()
	machines[appAddress] = &MockMachine{AdvanceVal: advanceRes, AdvanceErr: nil}

	repository := &MockRepository{
		GetInputsVal:    map[Address][]*Input{appAddress: randomInputs(1)},
		GetInputsErr:    nil,
		StoreResultsErr: nil,
	}

	advancer, err := New(machines, repository)
	require.NotNil(advancer)
	require.Nil(err)

	err = advancer.Run(context.Background())
	require.Nil(err)

	require.Len(repository.Stored, 1)
	require.Equal(advanceRes, repository.Stored[0])
}

// ------------------------------------------------------------------------------------------------

type MockMachine struct {
	AdvanceVal *nodemachine.AdvanceResult
	AdvanceErr error
}

func (mock *MockMachine) Advance(_ context.Context, _ []byte) (*nodemachine.AdvanceResult, error) {
	return mock.AdvanceVal, mock.AdvanceErr
}

// ------------------------------------------------------------------------------------------------

type MockRepository struct {
	GetInputsVal    map[Address][]*Input
	GetInputsErr    error
	StoreResultsErr error

	Stored []*nodemachine.AdvanceResult
}

func (mock *MockRepository) GetInputs(
	_ context.Context,
	appAddresses []Address,
) (map[Address][]*Input, error) {
	return mock.GetInputsVal, mock.GetInputsErr
}

func (mock *MockRepository) StoreResults(
	_ context.Context,
	input *Input,
	res *nodemachine.AdvanceResult,
) error {
	mock.Stored = append(mock.Stored, res)
	return mock.StoreResultsErr
}

// ------------------------------------------------------------------------------------------------

func randomAddress() Address {
	address := make([]byte, 20)
	_, err := crand.Read(address)
	if err != nil {
		panic(err)
	}
	return Address(address)
}

func randomHash() Hash {
	hash := make([]byte, 32)
	_, err := crand.Read(hash)
	if err != nil {
		panic(err)
	}
	return Hash(hash)
}

func randomBytes() []byte {
	size := mrand.Intn(100) + 1
	bytes := make([]byte, size)
	_, err := crand.Read(bytes)
	if err != nil {
		panic(err)
	}
	return bytes
}

func randomSliceOfBytes() [][]byte {
	size := mrand.Intn(10) + 1
	slice := make([][]byte, size)
	for i := 0; i < size; i++ {
		slice[i] = randomBytes()
	}
	return slice
}

func randomInputs(size int) []*Input {
	slice := make([]*Input, size)
	for i := 0; i < size; i++ {
		slice[i] = &Input{Id: uint64(i), RawData: randomBytes()}
	}
	return slice

}

func randomAdvanceResult() *nodemachine.AdvanceResult {
	return &nodemachine.AdvanceResult{
		Status:      InputStatusAccepted,
		Outputs:     randomSliceOfBytes(),
		Reports:     randomSliceOfBytes(),
		OutputsHash: randomHash(),
		MachineHash: randomHash(),
	}
}
