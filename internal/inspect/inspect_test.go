// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inspect

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/manager"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/services"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/suite"
)

const TestTimeout = 5 * time.Second

func TestInspect(t *testing.T) {
	suite.Run(t, new(InspectSuite))
}

type InspectSuite struct {
	suite.Suite
	ServicePort int
	ServiceAddr string
}

func (s *InspectSuite) SetupSuite() {
	s.ServicePort = 5555
}

func (s *InspectSuite) SetupTest() {
	s.ServicePort++
	s.ServiceAddr = fmt.Sprintf("127.0.0.1:%v", s.ServicePort)
}

func (s *InspectSuite) TestPostOk() {
	inspect, app, payload := s.setup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router := http.NewServeMux()
	router.Handle("/inspect/{dapp}", inspect)
	httpService := services.HttpService{Name: "http", Address: s.ServiceAddr, Handler: router}

	result := make(chan error, 1)
	ready := make(chan struct{}, 1)
	go func() {
		result <- httpService.Start(ctx, ready, service.NewLogger(slog.LevelDebug, true))
	}()

	select {
	case <-ready:
	case <-time.After(TestTimeout):
		s.FailNow("timed out waiting for HttpService to be ready")
	}

	resp, err := http.Post(fmt.Sprintf("http://%v/inspect/%v", s.ServiceAddr, app.IApplicationAddress.Hex()),
		"application/octet-stream",
		bytes.NewBuffer(payload.Bytes()))
	if err != nil {
		s.FailNow(err.Error())
	}
	s.assertResponse(resp, payload.Hex())
}

func (s *InspectSuite) TestPostWithNameOk() {
	inspect, app, payload := s.setup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router := http.NewServeMux()
	router.Handle("/inspect/{dapp}", inspect)
	httpService := services.HttpService{Name: "http", Address: s.ServiceAddr, Handler: router}

	result := make(chan error, 1)
	ready := make(chan struct{}, 1)
	go func() {
		result <- httpService.Start(ctx, ready, service.NewLogger(slog.LevelDebug, true))
	}()

	select {
	case <-ready:
	case <-time.After(TestTimeout):
		s.FailNow("timed out waiting for HttpService to be ready")
	}

	resp, err := http.Post(fmt.Sprintf("http://%s/inspect/%s", s.ServiceAddr, app.Name),
		"application/octet-stream",
		bytes.NewBuffer(payload.Bytes()))
	if err != nil {
		s.FailNow(err.Error())
	}
	s.assertResponse(resp, payload.Hex())
}

func (s *InspectSuite) TestPostNoApp() {
	inspect, _, payload := s.setup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router := http.NewServeMux()
	router.Handle("/inspect/{dapp}", inspect)
	httpService := services.HttpService{Name: "http", Address: s.ServiceAddr, Handler: router}

	result := make(chan error, 1)
	ready := make(chan struct{}, 1)
	go func() {
		result <- httpService.Start(ctx, ready, service.NewLogger(slog.LevelDebug, true))
	}()

	select {
	case <-ready:
	case <-time.After(TestTimeout):
		s.FailNow("timed out waiting for HttpService to be ready")
	}

	resp, err := http.Post(fmt.Sprintf("http://%s/inspect/%s", s.ServiceAddr, "Aloha"),
		"application/octet-stream",
		bytes.NewBuffer(payload.Bytes()))
	s.Require().Nil(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)

	resp, err = http.Post(fmt.Sprintf("http://%s/inspect/%s", s.ServiceAddr,
		"0x1000000000000000000000000000000000000000"),
		"application/octet-stream",
		bytes.NewBuffer(payload.Bytes()))
	s.Require().Nil(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
}

func (s *InspectSuite) TestPostMachineNotReady() {
	// App exists in the repository but has no machine in the machines map.
	// This simulates the startup window where the advancer hasn't created
	// the machine instance yet. Should return 503 Service Unavailable.
	app := &Application{
		ID:                  42,
		IApplicationAddress: randomAddress(),
		Name:                "app-no-machine",
	}
	repo := newMockRepository()
	repo.apps = append(repo.apps, app)
	machines := newMockMachines() // no machine added for app ID 42

	inspect := &Inspector{
		repository:       repo,
		IInspectMachines: machines,
		Logger:           service.NewLogger(slog.LevelDebug, true),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router := http.NewServeMux()
	router.Handle("/inspect/{dapp}", inspect)
	httpService := services.HttpService{Name: "http", Address: s.ServiceAddr, Handler: router}

	ready := make(chan struct{}, 1)
	go func() {
		_ = httpService.Start(ctx, ready, service.NewLogger(slog.LevelDebug, true))
	}()

	select {
	case <-ready:
	case <-time.After(TestTimeout):
		s.FailNow("timed out waiting for HttpService to be ready")
	}

	// Query by name
	respByName, err := http.Post(fmt.Sprintf("http://%s/inspect/%s", s.ServiceAddr, app.Name),
		"application/octet-stream",
		bytes.NewBuffer([]byte("hello")))
	s.Require().Nil(err)
	defer respByName.Body.Close()
	s.Equal(http.StatusServiceUnavailable, respByName.StatusCode)

	// Query by address
	respByAddr, err := http.Post(fmt.Sprintf("http://%s/inspect/%s", s.ServiceAddr, app.IApplicationAddress.Hex()),
		"application/octet-stream",
		bytes.NewBuffer([]byte("hello")))
	s.Require().Nil(err)
	defer respByAddr.Body.Close()
	s.Equal(http.StatusServiceUnavailable, respByAddr.StatusCode)
}

func (s *InspectSuite) TestPostMaxPayloadSize() {
	inspect, app, _ := s.setup()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.startServer(ctx, inspect)

	// A payload exactly at the max size should be accepted.
	payload := make([]byte, maxPayloadSize)
	_, err := crand.Read(payload)
	s.Require().NoError(err)

	resp, err := http.Post(
		fmt.Sprintf("http://%s/inspect/%s", s.ServiceAddr, app.IApplicationAddress.Hex()),
		"application/octet-stream",
		bytes.NewReader(payload))
	s.Require().NoError(err)
	defer resp.Body.Close()
	s.Equal(http.StatusOK, resp.StatusCode)

	var r InspectResponse
	err = json.NewDecoder(resp.Body).Decode(&r)
	s.Require().NoError(err)
	s.Equal("Accepted", r.Status)
	s.Require().Len(r.Reports, 1)
}

func (s *InspectSuite) TestPostPayloadTooLarge() {
	inspect, app, _ := s.setup()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.startServer(ctx, inspect)

	// A payload one byte over the max size should be rejected.
	payload := make([]byte, maxPayloadSize+1)
	_, err := crand.Read(payload)
	s.Require().NoError(err)

	resp, err := http.Post(
		fmt.Sprintf("http://%s/inspect/%s", s.ServiceAddr, app.IApplicationAddress.Hex()),
		"application/octet-stream",
		bytes.NewReader(payload))
	s.Require().NoError(err)
	defer resp.Body.Close()
	s.Equal(http.StatusRequestEntityTooLarge, resp.StatusCode)
}

func (s *InspectSuite) startServer(ctx context.Context, inspect *Inspector) {
	router := http.NewServeMux()
	router.Handle("/inspect/{dapp}", inspect)
	httpService := services.HttpService{Name: "http", Address: s.ServiceAddr, Handler: router}

	ready := make(chan struct{}, 1)
	go func() {
		_ = httpService.Start(ctx, ready, service.NewLogger(slog.LevelDebug, true))
	}()

	select {
	case <-ready:
	case <-time.After(TestTimeout):
		s.FailNow("timed out waiting for HttpService to be ready")
	}
}

func (s *InspectSuite) setup() (*Inspector, *Application, common.Hash) {
	m := newMockMachine(1)
	repo := newMockRepository()
	repo.apps = append(repo.apps, m.application)
	machines := newMockMachines()
	machines.Map[1] = *m
	inspect := &Inspector{
		repository:       repo,
		IInspectMachines: machines,
		Logger:           service.NewLogger(slog.LevelDebug, true),
	}
	payload := randomHash()
	return inspect, m.application, payload
}

func (s *InspectSuite) assertResponse(resp *http.Response, payload string) {
	s.Equal(http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var r InspectResponse
	err := json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		s.FailNow("failed to read response body. ", err)
	}
	s.Equal(payload, r.Reports[0].Payload)
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

func (mock *MachinesMock) GetMachine(appId int64) (manager.MachineInstance, bool) {
	machine, exists := mock.Map[appId]
	if !exists {
		return nil, false
	}
	return &machine, exists
}

// ------------------------------------------------------------------------------------------------

type MockMachine struct {
	application *Application
}

func (mock *MockMachine) Inspect(
	_ context.Context,
	query []byte,
) (*InspectResult, error) {
	var res InspectResult
	var reports [][]byte

	reports = append(reports, query)
	res.Accepted = true
	res.ProcessedInputs = 0
	res.Error = nil
	res.Reports = reports

	return &res, nil
}

func (mock *MockMachine) Advance(
	_ context.Context,
	input []byte,
	_ uint64,
	_ uint64,
	_ bool,
) (*AdvanceResult, error) {
	// Not used in inspect tests, but needed to satisfy the interface
	return nil, nil
}

func (mock *MockMachine) Application() *Application {
	return mock.application
}

func (mock *MockMachine) ProcessedInputs() uint64 {
	return 0
}

func (m *MockMachine) OutputsProof(ctx context.Context) (*OutputsProof, error) {
	return nil, nil
}

func (mock *MockMachine) Synchronize(ctx context.Context, repo manager.MachineRepository, batchSize uint64) error {
	// Not used in inspect tests, but needed to satisfy the interface
	return nil
}

func (mock *MockMachine) CreateSnapshot(ctx context.Context, processedInputs uint64, path string) error {
	// Not used in inspect tests, but needed to satisfy the interface
	return nil
}

// Retrieves the hash of the current machine state
func (m *MockMachine) Hash(ctx context.Context) ([32]byte, error) {
	// Not used in inspect tests, but needed to satisfy the interface
	return [32]byte{}, nil
}

func (mock *MockMachine) Close() error {
	// Not used in inspect tests, but needed to satisfy the interface
	return nil
}

func newMockMachine(id int64) *MockMachine {
	return &MockMachine{
		application: &Application{
			ID:                  id,
			IApplicationAddress: randomAddress(),
			Name:                fmt.Sprintf("app-%v", id),
		},
	}
}

// ------------------------------------------------------------------------------------------------

type MockRepository struct {
	apps []*Application
}

func (mock *MockRepository) GetApplication(ctx context.Context, nameOrAddress string) (*Application, error) {
	for _, app := range mock.apps {
		if app.Name == nameOrAddress || app.IApplicationAddress == common.HexToAddress(nameOrAddress) {
			return app, nil
		}
	}
	return nil, nil
}

func newMockRepository() *MockRepository {
	return &MockRepository{apps: []*Application{}}
}

// ------------------------------------------------------------------------------------------------

func randomAddress() common.Address {
	address := make([]byte, 20)
	_, err := crand.Read(address)
	if err != nil {
		panic(err)
	}
	return common.Address(address)
}

func randomHash() common.Hash {
	hash := make([]byte, 32)
	_, err := crand.Read(hash)
	if err != nil {
		panic(err)
	}
	return common.Hash(hash)
}
