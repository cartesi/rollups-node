// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	mrand "math/rand"
	"testing"

	"github.com/cartesi/rollups-node/internal/advancer/machines"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/nodemachine"

	"github.com/stretchr/testify/suite"
)

func TestAdvancer(t *testing.T) {
	suite.Run(t, new(AdvancerSuite))
}

type AdvancerSuite struct{ suite.Suite }

func (s *AdvancerSuite) TestNew() {
	s.Run("Ok", func() {
		require := s.Require()
		machines := newMockMachines()
		machines.Map[randomAddress()] = &MockMachine{}
		var repository Repository = &MockRepository{}
		advancer, err := New(machines, repository)
		require.NotNil(advancer)
		require.Nil(err)
	})

	s.Run("InvalidMachines", func() {
		require := s.Require()
		var machines Machines = nil
		var repository Repository = &MockRepository{}
		advancer, err := New(machines, repository)
		require.Nil(advancer)
		require.Error(err)
		require.Equal(ErrInvalidMachines, err)
	})

	s.Run("InvalidRepository", func() {
		require := s.Require()
		machines := newMockMachines()
		machines.Map[randomAddress()] = &MockMachine{}
		var repository Repository = nil
		advancer, err := New(machines, repository)
		require.Nil(advancer)
		require.Error(err)
		require.Equal(ErrInvalidRepository, err)
	})
}

func (s *AdvancerSuite) TestPoller() {
	s.T().Skip("TODO")
}

func (s *AdvancerSuite) TestRun() {
	s.Run("Ok", func() {
		require := s.Require()

		machines := newMockMachines()
		app1 := randomAddress()
		machines.Map[app1] = &MockMachine{}
		app2 := randomAddress()
		machines.Map[app2] = &MockMachine{}
		res1 := randomAdvanceResult()
		res2 := randomAdvanceResult()
		res3 := randomAdvanceResult()

		repository := &MockRepository{
			GetInputsReturn: map[Address][]*Input{
				app1: {
					{Id: 1, RawData: marshal(res1)},
					{Id: 2, RawData: marshal(res2)},
				},
				app2: {
					{Id: 5, RawData: marshal(res3)},
				},
			},
		}

		advancer, err := New(machines, repository)
		require.NotNil(advancer)
		require.Nil(err)

		err = advancer.Step(context.Background())
		require.Nil(err)

		require.Len(repository.StoredResults, 3)
	})

	s.Run("Error/UpdateEpochs", func() {
		s.T().Skip("TODO")
	})

	// NOTE: missing more test cases
}

func (s *AdvancerSuite) TestProcess() {
	setup := func() (Machines, *MockRepository, *Advancer, Address) {
		app := randomAddress()
		machines := newMockMachines()
		machines.Map[app] = &MockMachine{}
		repository := &MockRepository{}
		advancer := &Advancer{machines, repository}
		return machines, repository, advancer, app
	}

	s.Run("Ok", func() {
		require := s.Require()

		_, repository, advancer, app := setup()
		inputs := []*Input{
			{Id: 1, RawData: marshal(randomAdvanceResult())},
			{Id: 2, RawData: marshal(randomAdvanceResult())},
			{Id: 3, RawData: marshal(randomAdvanceResult())},
			{Id: 4, RawData: marshal(randomAdvanceResult())},
			{Id: 5, RawData: marshal(randomAdvanceResult())},
			{Id: 6, RawData: marshal(randomAdvanceResult())},
			{Id: 7, RawData: marshal(randomAdvanceResult())},
		}

		err := advancer.process(context.Background(), app, inputs)
		require.Nil(err)
		require.Len(repository.StoredResults, 7)
	})

	s.Run("Panic", func() {
		s.Run("ErrApp", func() {
			require := s.Require()

			invalidApp := randomAddress()
			_, _, advancer, _ := setup()
			inputs := randomInputs(3)

			expected := fmt.Sprintf("%v %v", ErrNoApp, invalidApp)
			require.PanicsWithError(expected, func() {
				_ = advancer.process(context.Background(), invalidApp, inputs)
			})
		})

		s.Run("ErrInputs", func() {
			require := s.Require()

			_, _, advancer, app := setup()
			inputs := []*Input{}

			require.PanicsWithValue(ErrNoInputs, func() {
				_ = advancer.process(context.Background(), app, inputs)
			})
		})
	})

	s.Run("Error", func() {
		s.Run("Advance", func() {
			require := s.Require()

			_, repository, advancer, app := setup()
			inputs := []*Input{
				{Id: 1, RawData: marshal(randomAdvanceResult())},
				{Id: 2, RawData: []byte("advance error")},
				{Id: 3, RawData: []byte("unreachable")},
			}

			err := advancer.process(context.Background(), app, inputs)
			require.Errorf(err, "advance error")
			require.Len(repository.StoredResults, 1)
		})

		s.Run("StoreAdvance", func() {
			require := s.Require()

			_, repository, advancer, app := setup()
			inputs := []*Input{
				{Id: 1, RawData: marshal(randomAdvanceResult())},
				{Id: 2, RawData: []byte("unreachable")},
			}
			repository.StoreAdvanceError = errors.New("store-advance error")

			err := advancer.process(context.Background(), app, inputs)
			require.Errorf(err, "store-advance error")
			require.Len(repository.StoredResults, 1)
		})
	})
}

// ------------------------------------------------------------------------------------------------

type MockMachine struct{}

func (mock *MockMachine) Advance(
	_ context.Context,
	input []byte,
	_ uint64,
) (*nodemachine.AdvanceResult, error) {
	var res nodemachine.AdvanceResult
	err := json.Unmarshal(input, &res)
	if err != nil {
		return nil, errors.New(string(input))
	}
	return &res, nil
}

// ------------------------------------------------------------------------------------------------

type MachinesMock struct {
	Map map[Address]machines.AdvanceMachine
}

func newMockMachines() *MachinesMock {
	return &MachinesMock{
		Map: map[Address]machines.AdvanceMachine{},
	}
}

func (mock *MachinesMock) GetAdvanceMachine(app Address) machines.AdvanceMachine {
	return mock.Map[app]
}

func (mock *MachinesMock) Apps() []Address {
	return []Address{}
}

// ------------------------------------------------------------------------------------------------

type MockRepository struct {
	GetInputsReturn   map[Address][]*Input
	GetInputsError    error
	StoreAdvanceError error
	UpdateEpochsError error

	StoredResults []*nodemachine.AdvanceResult
}

func (mock *MockRepository) GetUnprocessedInputs(
	_ context.Context,
	appAddresses []Address,
) (map[Address][]*Input, error) {
	return mock.GetInputsReturn, mock.GetInputsError
}

func (mock *MockRepository) StoreAdvanceResult(
	_ context.Context,
	input *Input,
	res *nodemachine.AdvanceResult,
) error {
	mock.StoredResults = append(mock.StoredResults, res)
	return mock.StoreAdvanceError
}

func (mock *MockRepository) UpdateEpochs(_ context.Context, _ Address) error {
	return mock.UpdateEpochsError
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
	res := &nodemachine.AdvanceResult{
		Status:      InputStatusAccepted,
		Outputs:     randomSliceOfBytes(),
		Reports:     randomSliceOfBytes(),
		OutputsHash: randomHash(),
		MachineHash: new(Hash),
	}
	*res.MachineHash = randomHash()
	return res
}

func marshal(res *nodemachine.AdvanceResult) []byte {
	data, err := json.Marshal(*res)
	if err != nil {
		panic(err)
	}
	return data
}
