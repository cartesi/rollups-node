// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inspect

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/manager"
	. "github.com/cartesi/rollups-node/internal/model"
	inspectclient "github.com/cartesi/rollups-node/pkg/inspectclient"
	pkgmachine "github.com/cartesi/rollups-node/pkg/machine"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/suite"
)

func TestInspect(t *testing.T) {
	suite.Run(t, new(InspectSuite))
}

type InspectSuite struct {
	suite.Suite
}

func (s *InspectSuite) TestPostOk() {
	inspect, app, payload := s.setup()

	srv := s.startServer(inspect)
	defer srv.Close()

	resp, err := http.Post(fmt.Sprintf("%s/inspect/%s", srv.URL, app.IApplicationAddress.Hex()),
		"application/octet-stream",
		bytes.NewBuffer(payload.Bytes()))
	if err != nil {
		s.FailNow(err.Error())
	}
	s.assertResponse(resp, payload.Hex())
}

func (s *InspectSuite) TestPostWithNameOk() {
	inspect, app, payload := s.setup()

	srv := s.startServer(inspect)
	defer srv.Close()

	resp, err := http.Post(fmt.Sprintf("%s/inspect/%s", srv.URL, app.Name),
		"application/octet-stream",
		bytes.NewBuffer(payload.Bytes()))
	if err != nil {
		s.FailNow(err.Error())
	}
	s.assertResponse(resp, payload.Hex())
}

func (s *InspectSuite) TestPostNoApp() {
	inspect, _, payload := s.setup()

	srv := s.startServer(inspect)
	defer srv.Close()

	resp, err := http.Post(fmt.Sprintf("%s/inspect/%s", srv.URL, "Aloha"),
		"application/octet-stream",
		bytes.NewBuffer(payload.Bytes()))
	s.Require().Nil(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)

	resp, err = http.Post(fmt.Sprintf("%s/inspect/%s", srv.URL,
		"0x1000000000000000000000000000000000000000"),
		"application/octet-stream",
		bytes.NewBuffer(payload.Bytes()))
	s.Require().Nil(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
}

func (s *InspectSuite) TestPostMachineNotReady() {
	app := &Application{
		ID:                  42,
		IApplicationAddress: randomAddress(),
		Name:                "app-no-machine",
	}
	repo := newMockRepository()
	repo.apps = append(repo.apps, app)
	machines := newMockMachines()

	inspect := &Inspector{
		repository:       repo,
		IInspectMachines: machines,
		Logger:           service.NewLogger(slog.LevelDebug, true),
	}

	srv := s.startServer(inspect)
	defer srv.Close()

	respByName, err := http.Post(fmt.Sprintf("%s/inspect/%s", srv.URL, app.Name),
		"application/octet-stream",
		bytes.NewBuffer([]byte("hello")))
	s.Require().Nil(err)
	defer respByName.Body.Close()
	s.Equal(http.StatusServiceUnavailable, respByName.StatusCode)

	respByAddr, err := http.Post(fmt.Sprintf("%s/inspect/%s", srv.URL, app.IApplicationAddress.Hex()),
		"application/octet-stream",
		bytes.NewBuffer([]byte("hello")))
	s.Require().Nil(err)
	defer respByAddr.Body.Close()
	s.Equal(http.StatusServiceUnavailable, respByAddr.StatusCode)
}

func (s *InspectSuite) TestPostForeclosedMachineUnavailable() {
	app := &Application{
		ID:                  42,
		IApplicationAddress: randomAddress(),
		Name:                "app-foreclosed",
		Status:              ApplicationStatus_OK,
		ForecloseBlock:      100,
	}
	repo := newMockRepository()
	repo.apps = append(repo.apps, app)
	machines := newMockMachines()

	inspect := &Inspector{
		repository:       repo,
		IInspectMachines: machines,
		Logger:           service.NewLogger(slog.LevelDebug, true),
	}

	srv := s.startServer(inspect)
	defer srv.Close()

	resp, err := http.Post(fmt.Sprintf("%s/inspect/%s", srv.URL, app.Name),
		"application/octet-stream",
		bytes.NewBuffer([]byte("hello")))
	s.Require().Nil(err)
	defer resp.Body.Close()
	s.Equal(http.StatusServiceUnavailable, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	s.Require().Nil(err)
	s.Contains(string(body), "Application was foreclosed; machine unavailable")
}

func (s *InspectSuite) TestPostTerminalApplicationUnavailable() {
	for _, status := range ApplicationStatusAllValues {
		if !status.IsTerminal() {
			continue
		}
		s.Run(status.String(), func() {
			inspect, app, _ := s.setup()
			app.Status = status
			request := httptest.NewRequest(
				http.MethodPost,
				"/inspect/"+app.Name,
				bytes.NewBufferString("query"),
			)
			request.SetPathValue("dapp", app.Name)
			recorder := httptest.NewRecorder()

			inspect.ServeHTTP(recorder, request)

			s.Equal(http.StatusServiceUnavailable, recorder.Code)
			s.Contains(recorder.Body.String(), "Application is terminal; inspect unavailable")
		})
	}
}

func (s *InspectSuite) TestPostMachineClosedDuringInspectIsUnavailable() {
	inspect, app, _ := s.setup()
	machine := inspect.IInspectMachines.(*MachinesMock).Map[app.ID]
	machine.inspectError = manager.ErrMachineClosed
	inspect.IInspectMachines.(*MachinesMock).Map[app.ID] = machine
	request := httptest.NewRequest(
		http.MethodPost,
		"/inspect/"+app.Name,
		bytes.NewBufferString("query"),
	)
	request.SetPathValue("dapp", app.Name)
	recorder := httptest.NewRecorder()

	inspect.ServeHTTP(recorder, request)

	s.Equal(http.StatusServiceUnavailable, recorder.Code)
	s.Contains(recorder.Body.String(), "Machine not ready")
}

func (s *InspectSuite) TestPostMaxPayloadSize() {
	inspect, app, _ := s.setup()

	srv := s.startServer(inspect)
	defer srv.Close()

	payload := make([]byte, maxPayloadSize)
	_, err := crand.Read(payload)
	s.Require().NoError(err)

	resp, err := http.Post(
		fmt.Sprintf("%s/inspect/%s", srv.URL, app.IApplicationAddress.Hex()),
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

	srv := s.startServer(inspect)
	defer srv.Close()

	payload := make([]byte, maxPayloadSize+1)
	_, err := crand.Read(payload)
	s.Require().NoError(err)

	resp, err := http.Post(
		fmt.Sprintf("%s/inspect/%s", srv.URL, app.IApplicationAddress.Hex()),
		"application/octet-stream",
		bytes.NewReader(payload))
	s.Require().NoError(err)
	defer resp.Body.Close()
	s.Equal(http.StatusRequestEntityTooLarge, resp.StatusCode)
}

func (s *InspectSuite) TestPostResponseMatchesGeneratedClientContract() {
	tests := []struct {
		name              string
		result            manager.InspectResult
		wantStatus        inspectclient.CompletionStatus
		wantError         string
		wantExceptionData []byte
	}{
		{
			name: "accepted",
			result: manager.InspectResult{
				Status:          pkgmachine.CompletionStatusAccepted,
				Reports:         [][]byte{{0xde, 0xad}, {}},
				ProcessedInputs: 17,
			},
			wantStatus: inspectclient.Accepted,
		},
		{
			name: "rejected",
			result: manager.InspectResult{
				Status:          pkgmachine.CompletionStatusRejected,
				Reports:         [][]byte{{0xbe, 0xef}},
				ProcessedInputs: 23,
			},
			wantStatus: inspectclient.Rejected,
		},
		{
			name: "exception",
			result: manager.InspectResult{
				Status:          pkgmachine.CompletionStatusException,
				ExceptionData:   []byte{0xff, 0x00, 0x80},
				Reports:         [][]byte{{0xca, 0xfe}},
				ProcessedInputs: 42,
			},
			wantStatus:        inspectclient.Exception,
			wantError:         "The machine raised an exception while inspecting",
			wantExceptionData: []byte{0xff, 0x00, 0x80},
		},
		{
			name: "halted",
			result: manager.InspectResult{
				Status:          pkgmachine.CompletionStatusHalted,
				Reports:         [][]byte{{0xfa, 0xce}},
				ProcessedInputs: 51,
			},
			wantStatus: inspectclient.MachineHalted,
		},
		{
			name: "overflow",
			result: manager.InspectResult{
				Status:          pkgmachine.CompletionStatusOverflow,
				Reports:         [][]byte{{0xab, 0xcd}},
				ProcessedInputs: 52,
			},
			wantStatus: inspectclient.Failed,
			wantError:  inspectFailureMessage,
		},
		{
			name: "unexpected yield",
			result: manager.InspectResult{
				Status:          pkgmachine.CompletionStatusUnexpectedYield,
				Reports:         [][]byte{{0xab, 0xce}},
				ProcessedInputs: 53,
			},
			wantStatus: inspectclient.Failed,
			wantError:  inspectFailureMessage,
		},
		{
			name: "failed",
			result: manager.InspectResult{
				Status:          pkgmachine.CompletionStatusUnknown,
				Reports:         [][]byte{{0xba, 0xdd}},
				ProcessedInputs: 63,
				Error:           errors.New("backend disconnected"),
			},
			wantStatus: inspectclient.Failed,
			wantError:  inspectFailureMessage,
		},
	}

	for _, test := range tests {
		s.Run(test.name, func() {
			inspect, app := s.setupWithInspectResult(test.result)
			app.Status = ApplicationStatus_OK
			request := httptest.NewRequest(http.MethodPost, "/inspect/"+app.Name,
				bytes.NewBufferString("query"))
			request.SetPathValue("dapp", app.Name)
			recorder := httptest.NewRecorder()
			inspect.ServeHTTP(recorder, request)
			s.Equal(http.StatusOK, recorder.Code)

			var got inspectclient.InspectResult
			s.Require().NoError(json.NewDecoder(recorder.Body).Decode(&got))
			s.Equal(test.wantStatus, got.Status)
			s.EqualValues(test.result.ProcessedInputs, got.ProcessedInputCount)
			s.Require().Len(got.Reports, len(test.result.Reports))
			for i, report := range test.result.Reports {
				s.Equal(fmt.Sprintf("0x%x", report), got.Reports[i].Payload)
			}
			if test.wantError == "" {
				s.Nil(got.Error)
			} else {
				s.Require().NotNil(got.Error)
				s.Equal(test.wantError, *got.Error)
			}
			if test.wantExceptionData == nil {
				s.Nil(got.ExceptionData)
			} else {
				s.Require().NotNil(got.ExceptionData)
				s.Equal(fmt.Sprintf("0x%x", test.wantExceptionData), *got.ExceptionData)
			}
			s.Equal(ApplicationStatus_OK, app.Status, "inspect limits must not change application status")
		})
	}
}

func (s *InspectSuite) TestCycleLimitIsSanitizedFailedResultWithoutApplicationFailure() {
	detailedErr := fmt.Errorf(
		"inspect stopped at absolute_mcycle=123456 with configured_cap=789: %w",
		pkgmachine.ErrReachedLimitMcycle,
	)
	inspect, app := s.setupWithInspectResult(manager.InspectResult{
		Status:          pkgmachine.CompletionStatusUnknown,
		ProcessedInputs: 42,
		Error:           detailedErr,
	})
	var logs bytes.Buffer
	inspect.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	app.Status = ApplicationStatus_OK
	req := httptest.NewRequest(http.MethodPost, "/inspect/"+app.Name, bytes.NewBufferString("query"))
	req.SetPathValue("dapp", app.Name)
	recorder := httptest.NewRecorder()

	inspect.ServeHTTP(recorder, req)

	s.Equal(http.StatusOK, recorder.Code)
	var got inspectclient.InspectResult
	s.Require().NoError(json.NewDecoder(recorder.Body).Decode(&got))
	s.Equal(inspectclient.Failed, got.Status)
	s.Require().NotNil(got.Error)
	s.Equal(inspectFailureMessage, *got.Error)
	s.NotContains(*got.Error, "absolute_mcycle")
	s.NotContains(*got.Error, "configured_cap")
	s.Contains(logs.String(), "absolute_mcycle=123456")
	s.Contains(logs.String(), "configured_cap=789")
	s.EqualValues(42, got.ProcessedInputCount)
	s.Equal(ApplicationStatus_OK, app.Status)
}

func (s *InspectSuite) TestGuestExceptionUsesDebugLogLevel() {
	inspect, app := s.setupWithInspectResult(manager.InspectResult{
		Status:          pkgmachine.CompletionStatusException,
		ExceptionData:   []byte("guest exception details"),
		ProcessedInputs: 42,
	})
	var logs bytes.Buffer
	inspect.Logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	req := httptest.NewRequest(http.MethodPost, "/inspect/"+app.Name, bytes.NewBufferString("query"))
	req.SetPathValue("dapp", app.Name)
	recorder := httptest.NewRecorder()

	inspect.ServeHTTP(recorder, req)

	s.Equal(http.StatusOK, recorder.Code)
	s.Contains(logs.String(), "level=DEBUG")
	s.Contains(logs.String(), "Machine returned a guest inspect exception")
	s.NotContains(logs.String(), "level=WARN")
	s.NotContains(logs.String(), "guest exception details")
}

func (s *InspectSuite) startServer(inspect *Inspector) *httptest.Server {
	router := http.NewServeMux()
	router.Handle("/inspect/{dapp}", inspect)
	return httptest.NewServer(router)
}

func (s *InspectSuite) setup() (*Inspector, *Application, common.Hash) {
	payload := randomHash()
	inspect, app := s.setupWithInspectResult(manager.InspectResult{
		Status:  pkgmachine.CompletionStatusAccepted,
		Reports: [][]byte{payload.Bytes()},
	})
	return inspect, app, payload
}

func (s *InspectSuite) setupWithInspectResult(result manager.InspectResult) (*Inspector, *Application) {
	m := newMockMachine(1)
	m.inspectResult = &result
	repo := newMockRepository()
	repo.apps = append(repo.apps, m.application)
	machines := newMockMachines()
	machines.Map[1] = *m
	inspect := &Inspector{
		repository:       repo,
		IInspectMachines: machines,
		Logger:           service.NewLogger(slog.LevelDebug, true),
	}
	return inspect, m.application
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
	application   *Application
	inspectResult *manager.InspectResult
	inspectError  error
}

func (mock *MockMachine) Inspect(
	_ context.Context,
	query []byte,
) (*manager.InspectResult, error) {
	if mock.inspectError != nil {
		return nil, mock.inspectError
	}
	if mock.inspectResult != nil {
		result := *mock.inspectResult
		return &result, nil
	}

	var res manager.InspectResult
	var reports [][]byte

	reports = append(reports, query)
	res.Status = pkgmachine.CompletionStatusAccepted
	res.ProcessedInputs = 0
	res.Error = nil
	res.Reports = reports

	return &res, nil
}

// Not used in inspect tests, but needed to satisfy the interface
func (mock *MockMachine) Advance(
	_ context.Context,
	input []byte,
	_ uint64,
	_ uint64,
	_ bool,
) (*AdvanceResult, error) {
	return nil, nil
}

func (mock *MockMachine) Application() *Application {
	return mock.application
}

func (mock *MockMachine) ProcessedInputs() uint64 {
	return 0
}

func (mock *MockMachine) StateProof(_ context.Context) (*StateProof, error) {
	return nil, nil
}

// Not used in inspect tests, but needed to satisfy the interface
func (mock *MockMachine) CreateSnapshot(ctx context.Context, processedInputs uint64, path string) error {
	return nil
}

// Retrieves the hash of the current machine state
func (m *MockMachine) Hash(ctx context.Context) ([32]byte, error) {
	return [32]byte{}, nil
}

// Not used in inspect tests, but needed to satisfy the interface
func (mock *MockMachine) Close() error {
	return nil
}

func newMockMachine(id int64) *MockMachine {
	return &MockMachine{
		application: &Application{
			ID:                  id,
			IApplicationAddress: randomAddress(),
			Name:                fmt.Sprintf("app-%v", id),
			ExecutionParameters: ExecutionParameters{
				InspectMaxDeadline: 10 * time.Second,
			},
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
