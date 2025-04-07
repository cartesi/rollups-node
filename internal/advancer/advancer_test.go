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

	"github.com/cartesi/rollups-node/internal/manager"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/suite"
)

func TestAdvancer(t *testing.T) {
	suite.Run(t, new(AdvancerSuite))
}

type AdvancerSuite struct{ suite.Suite }

func newMock(m *MachinesMock, r *MockRepository) (*Service, error) {
	s := &Service{
		machineManager: m,
		repository:     r,
	}
	serviceArgs := &service.CreateInfo{Name: "advancer", Impl: s}
	err := service.Create(context.Background(), serviceArgs, &s.Service)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *AdvancerSuite) TestStep() {
	s.Run("Ok", func() {
		require := s.Require()

		machines := newMockMachines()
		app1 := newMockMachine(1)
		app2 := newMockMachine(2)
		machines.Map[1] = *app1
		machines.Map[2] = *app2
		res1 := randomAdvanceResult(1)
		res2 := randomAdvanceResult(2)
		res3 := randomAdvanceResult(3)

		repository := &MockRepository{
			GetInputsReturn: map[common.Address][]*Input{
				app1.Application.IApplicationAddress: {
					newInput(app1.Application.ID, 0, 0, marshal(res1)),
					newInput(app1.Application.ID, 0, 1, marshal(res2)),
				},
				app2.Application.IApplicationAddress: {
					newInput(app2.Application.ID, 0, 0, marshal(res3)),
				},
			},
		}

		advancer, err := newMock(machines, repository)
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
	setup := func() (*MachinesMock, *MockRepository, *Service, *MockMachine) {
		require := s.Require()

		machines := newMockMachines()
		app1 := newMockMachine(1)
		machines.Map[1] = *app1
		repository := &MockRepository{}
		advancer, err := newMock(machines, repository)
		require.Nil(err)
		return machines, repository, advancer, app1
	}

	s.Run("Ok", func() {
		require := s.Require()

		_, repository, advancer, app := setup()
		inputs := []*Input{
			newInput(app.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
			newInput(app.Application.ID, 0, 1, marshal(randomAdvanceResult(1))),
			newInput(app.Application.ID, 0, 2, marshal(randomAdvanceResult(2))),
			newInput(app.Application.ID, 0, 3, marshal(randomAdvanceResult(3))),
			newInput(app.Application.ID, 1, 4, marshal(randomAdvanceResult(4))),
			newInput(app.Application.ID, 1, 5, marshal(randomAdvanceResult(5))),
			newInput(app.Application.ID, 2, 6, marshal(randomAdvanceResult(6))),
		}

		err := advancer.processInputs(context.Background(), app.Application, inputs)
		require.Nil(err)
		require.Len(repository.StoredResults, 7)
	})

	s.Run("Noop", func() {
		s.Run("NoInputs", func() {
			require := s.Require()

			_, _, advancer, app := setup()
			inputs := []*Input{}

			err := advancer.processInputs(context.Background(), app.Application, inputs)
			require.Nil(err)
		})
	})

	s.Run("Error", func() {
		s.Run("ErrApp", func() {
			require := s.Require()

			invalidApp := Application{ID: 999}
			_, _, advancer, _ := setup()
			inputs := randomInputs(1, 0, 3)

			err := advancer.processInputs(context.Background(), &invalidApp, inputs)
			expected := fmt.Sprintf("%v: %v", ErrNoApp, invalidApp.ID)
			require.EqualError(err, expected)
		})

		s.Run("Advance", func() {
			require := s.Require()

			_, repository, advancer, app := setup()
			inputs := []*Input{
				newInput(app.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
				newInput(app.Application.ID, 0, 1, []byte("advance error")),
				newInput(app.Application.ID, 0, 2, []byte("unreachable")),
			}

			err := advancer.processInputs(context.Background(), app.Application, inputs)
			require.Error(err)
			require.Contains(err.Error(), "advance error")
			require.Len(repository.StoredResults, 1)
		})

		s.Run("StoreAdvance", func() {
			require := s.Require()

			_, repository, advancer, app := setup()
			inputs := []*Input{
				newInput(app.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
				newInput(app.Application.ID, 0, 1, []byte("unreachable")),
			}
			repository.StoreAdvanceError = errors.New("store-advance error")

			err := advancer.processInputs(context.Background(), app.Application, inputs)
			require.Error(err)
			require.Contains(err.Error(), "store-advance error")
			require.Len(repository.StoredResults, 1)
		})
	})
}

type MockMachine struct {
	Application *Application
}

func (mock *MockMachine) Advance(
	_ context.Context,
	input []byte,
	_ uint64,
) (*AdvanceResult, error) {
	var res AdvanceResult
	err := json.Unmarshal(input, &res)
	if err != nil {
		return nil, errors.New(string(input))
	}
	return &res, nil
}

func newMockMachine(id int64) *MockMachine {
	return &MockMachine{
		Application: &Application{
			ID:                  id,
			IApplicationAddress: randomAddress(),
		},
	}
}

// ------------------------------------------------------------------------------------------------

type MachinesMock struct {
	Map map[int64]MockMachine
}

func newMockMachines() *MachinesMock {
	return &MachinesMock{
		Map: map[int64]MockMachine{},
	}
}

func (mock *MachinesMock) GetMachine(appID int64) (manager.MachineInstance, bool) {
	machine, exists := mock.Map[appID]
	if !exists {
		return nil, false
	}

	// For testing purposes, we'll create a mock MachineInstance
	// that has the same Application but delegates the methods to our mock
	mockInstance := &MockMachineInstance{
		application: machine.Application,
		mockMachine: &machine,
	}

	return mockInstance, true
}

func (mock *MachinesMock) UpdateMachines(ctx context.Context) error {
	return nil
}

func (mock *MachinesMock) Applications() []*Application {
	keys := make([]*Application, len(mock.Map))
	i := 0
	for _, v := range mock.Map {
		keys[i] = v.Application
		i++
	}
	return keys
}

func (mock *MachinesMock) HasMachine(appID int64) bool {
	_, exists := mock.Map[appID]
	return exists
}

// MockMachineInstance is a test implementation of manager.MachineInstance
type MockMachineInstance struct {
	application *Application
	mockMachine *MockMachine
}

// Advance implements the AdvanceMachine interface for testing
func (m *MockMachineInstance) Advance(ctx context.Context, input []byte, index uint64) (*AdvanceResult, error) {
	return m.mockMachine.Advance(ctx, input, index)
}

// Inspect implements the InspectMachine interface for testing
func (m *MockMachineInstance) Inspect(ctx context.Context, query []byte) (*InspectResult, error) {
	// Not used in advancer tests, but needed to satisfy the interface
	return nil, nil
}

// Application returns the application associated with this machine
func (m *MockMachineInstance) Application() *Application {
	return m.application
}

// Synchronize implements the MachineInstance interface for testing
func (m *MockMachineInstance) Synchronize(ctx context.Context, repo manager.MachineRepository) error {
	// Not used in advancer tests, but needed to satisfy the interface
	return nil
}

// Close implements the MachineInstance interface for testing
func (m *MockMachineInstance) Close() error {
	// Not used in advancer tests, but needed to satisfy the interface
	return nil
}

// ------------------------------------------------------------------------------------------------

type MockRepository struct {
	GetInputsReturn             map[common.Address][]*Input
	GetInputsError              error
	StoreAdvanceError           error
	UpdateApplicationStateError error
	UpdateEpochsError           error
	UpdateEpochsCount           int64

	StoredResults []*AdvanceResult
}

func (mock *MockRepository) ListInputs(
	ctx context.Context,
	nameOrAddress string,
	f repository.InputFilter,
	p repository.Pagination,
) ([]*Input, uint64, error) {
	address := common.HexToAddress(nameOrAddress)
	return mock.GetInputsReturn[address], uint64(len(mock.GetInputsReturn[address])), mock.GetInputsError
}

func (mock *MockRepository) StoreAdvanceResult(
	_ context.Context,
	appID int64,
	res *AdvanceResult,
) error {
	mock.StoredResults = append(mock.StoredResults, res)
	return mock.StoreAdvanceError
}

func (mock *MockRepository) UpdateEpochsInputsProcessed(_ context.Context, nameOrAddress string) (int64, error) {
	return mock.UpdateEpochsCount, mock.UpdateEpochsError
}

func (mock *MockRepository) UpdateApplicationState(ctx context.Context, appID int64, state ApplicationState, reason *string) error {
	return mock.UpdateApplicationStateError
}

// ------------------------------------------------------------------------------------------------

func randomAddress() common.Address {
	address := make([]byte, 20)
	_, err := crand.Read(address)
	if err != nil {
		panic(err)
	}
	return common.BytesToAddress(address)
}

func randomHash() common.Hash {
	hash := make([]byte, 32)
	_, err := crand.Read(hash)
	if err != nil {
		panic(err)
	}
	return common.BytesToHash(hash)
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
	for i := range size {
		slice[i] = randomBytes()
	}
	return slice
}

func newInput(appId int64, epochIndex uint64, inputIndex uint64, data []byte) *Input {
	return &Input{
		EpochApplicationID: appId,
		EpochIndex:         epochIndex,
		Index:              inputIndex,
		RawData:            data,
	}
}

func randomInputs(appId int64, epochIndex uint64, size int) []*Input {
	slice := make([]*Input, size)
	for i := range size {
		slice[i] = newInput(appId, epochIndex, uint64(i), randomBytes())
	}
	return slice

}

func randomAdvanceResult(inputIndex uint64) *AdvanceResult {
	hash := randomHash()
	res := &AdvanceResult{
		InputIndex:  inputIndex,
		Status:      InputCompletionStatus_Accepted,
		Outputs:     randomSliceOfBytes(),
		Reports:     randomSliceOfBytes(),
		OutputsHash: randomHash(),
		MachineHash: &hash,
	}
	return res
}

func marshal(res *AdvanceResult) []byte {
	data, err := json.Marshal(*res)
	if err != nil {
		panic(err)
	}
	return data
}
