// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inspect

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/advancer/machines"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/nodemachine"
	"github.com/cartesi/rollups-node/internal/services"

	"github.com/stretchr/testify/suite"
)

const TestTimeout = 5 * time.Second

func TestAdvancer(t *testing.T) {
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

func (s *InspectSuite) TestNew() {
	s.Run("Ok", func() {
		require := s.Require()
		machines := newMockMachines()
		machines.Map[randomAddress()] = &MockMachine{}
		inspect, err := New(machines)
		require.NotNil(inspect)
		require.Nil(err)
	})

	s.Run("InvalidMachines", func() {
		require := s.Require()
		var machines Machines = nil
		inspect, err := New(machines)
		require.Nil(inspect)
		require.Error(err)
		require.Equal(ErrInvalidMachines, err)
	})
}

func (s *InspectSuite) TestGetOk() {
	inspect, app, payload := s.setup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router := http.NewServeMux()
	router.Handle("/test/{dapp}/{payload}", inspect)
	service := services.HttpService{Name: "http", Address: s.ServiceAddr, Handler: router}

	result := make(chan error, 1)
	ready := make(chan struct{}, 1)
	go func() {
		result <- service.Start(ctx, ready)
	}()

	select {
	case <-ready:
	case <-time.After(TestTimeout):
		s.FailNow("timed out waiting for HttpService to be ready")
	}

	resp, err := http.Get(fmt.Sprintf("http://%v/test/%v/%v",
		s.ServiceAddr,
		app.Hex(),
		payload.Hex()))
	if err != nil {
		s.FailNow(err.Error())
	}
	s.assertResponse(resp, payload.Hex())
}

func (s *InspectSuite) TestGetInvalidPayload() {
	inspect, app, _ := s.setup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router := http.NewServeMux()
	router.Handle("/test/{dapp}/{payload}", inspect)
	service := services.HttpService{Name: "http", Address: s.ServiceAddr, Handler: router}

	result := make(chan error, 1)
	ready := make(chan struct{}, 1)
	go func() {
		result <- service.Start(ctx, ready)
	}()

	select {
	case <-ready:
	case <-time.After(TestTimeout):
		s.FailNow("timed out waiting for HttpService to be ready")
	}

	resp, _ := http.Get(fmt.Sprintf("http://%v/test/%v/%v",
		s.ServiceAddr,
		app.Hex(),
		"qwertyuiop"))
	s.Equal(http.StatusBadRequest, resp.StatusCode)
	buf := new(strings.Builder)
	io.Copy(buf, resp.Body) //nolint: errcheck
	s.Require().Contains(buf.String(), "hex string without 0x prefix")
}

func (s *InspectSuite) TestPostOk() {
	inspect, app, payload := s.setup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router := http.NewServeMux()
	router.Handle("/test/{dapp}", inspect)
	service := services.HttpService{Name: "http", Address: s.ServiceAddr, Handler: router}

	result := make(chan error, 1)
	ready := make(chan struct{}, 1)
	go func() {
		result <- service.Start(ctx, ready)
	}()

	select {
	case <-ready:
	case <-time.After(TestTimeout):
		s.FailNow("timed out waiting for HttpService to be ready")
	}

	resp, err := http.Post(fmt.Sprintf("http://%v/test/%v", s.ServiceAddr, app.Hex()),
		"application/octet-stream",
		bytes.NewBuffer(payload.Bytes()))
	if err != nil {
		s.FailNow(err.Error())
	}
	s.assertResponse(resp, payload.Hex())
}

// Note: add more tests

func (s *InspectSuite) setup() (*Inspector, Address, Hash) {
	app := randomAddress()
	machines := newMockMachines()
	machines.Map[app] = &MockMachine{}
	inspect := &Inspector{machines}
	payload := randomHash()
	return inspect, app, payload
}

func (s *InspectSuite) assertResponse(resp *http.Response, payload string) {
	s.Equal(http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var r InspectResponse
	err := json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		s.FailNow("failed to read response body. ", err)
	}
	s.Equal(payload, r.Reports[0])
}

// ------------------------------------------------------------------------------------------------

type MachinesMock struct {
	Map map[Address]machines.InspectMachine
}

func newMockMachines() *MachinesMock {
	return &MachinesMock{
		Map: map[Address]machines.InspectMachine{},
	}
}

func (mock *MachinesMock) GetInspectMachine(app Address) machines.InspectMachine {
	return mock.Map[app]
}

// ------------------------------------------------------------------------------------------------

type MockMachine struct{}

func (mock *MockMachine) Inspect(
	_ context.Context,
	query []byte,
) (*nodemachine.InspectResult, error) {
	var res nodemachine.InspectResult
	var reports [][]byte
	var index *uint64 = new(uint64)
	*index = 0

	reports = append(reports, query)
	res.Accepted = true
	res.InputIndex = index
	res.Error = nil
	res.Reports = reports

	return &res, nil
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
