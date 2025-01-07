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

	"github.com/cartesi/rollups-node/internal/advancer/machines"
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

// FIXME: add more tests

func (s *InspectSuite) setup() (*Inspector, *Application, common.Hash) {
	m := newMockMachine(1)
	repo := newMockRepository()
	repo.apps = append(repo.apps, m.Application)
	machines := newMockMachines()
	machines.Map[1] = *m
	inspect := &Inspector{
		repository:       repo,
		IInspectMachines: machines,
		Logger:           service.NewLogger(slog.LevelDebug, true),
	}
	payload := randomHash()
	return inspect, m.Application, payload
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

func (mock *MachinesMock) GetInspectMachine(appId int64) (machines.InspectMachine, bool) {
	machine, exists := mock.Map[appId]
	return &machine, exists
}

// ------------------------------------------------------------------------------------------------

type MockMachine struct {
	Application *Application
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

func newMockMachine(id int64) *MockMachine {
	return &MockMachine{
		Application: &Application{
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
