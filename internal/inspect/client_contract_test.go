// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inspect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/inspectmessages"
	"github.com/cartesi/rollups-node/internal/manager"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/inspectclient"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/stretchr/testify/require"
)

type inspectHandlerTransport func(*http.Request) (*http.Response, error)

func (f inspectHandlerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type failedInspectBody struct{}

func (failedInspectBody) Read([]byte) (int, error) { return 0, errors.New("BODY_READ_CREDENTIAL") }

const inspectLogRequestID = "11111111-1111-4111-8111-111111111111"

func requireInspectRequestLogs(t *testing.T, logs *bytes.Buffer, levels []slog.Level) {
	t.Helper()
	decoder := json.NewDecoder(logs)
	var records []map[string]any
	for {
		var record map[string]any
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		records = append(records, record)
	}
	require.Len(t, records, len(levels), "adding the request ID must not add log events")
	for i, record := range records {
		require.Equal(t, levels[i].String(), record["level"])
		require.Equal(t, inspectLogRequestID, record["request_id"])
	}
}

func TestInspectorFixedHTTPMessagesReachProtectedClient(t *testing.T) {
	const applicationName = "app-1"
	for _, test := range []struct {
		message      string
		status       int
		configure    func(*MockMachine, *MachinesMock, *MockRepository)
		method, dapp string
		body         func() io.Reader
		levels       []slog.Level
	}{
		{message: inspectmessages.MissingApplicationAddress, status: http.StatusBadRequest, levels: []slog.Level{slog.LevelInfo}},
		{message: inspectmessages.MethodNotAllowed, status: http.StatusMethodNotAllowed,
			dapp: applicationName, method: http.MethodGet, levels: []slog.Level{slog.LevelInfo}},
		{message: inspectmessages.PayloadTooLarge, status: http.StatusRequestEntityTooLarge, dapp: applicationName,
			body:   func() io.Reader { return strings.NewReader(strings.Repeat("x", maxPayloadSize+1)) },
			levels: []slog.Level{slog.LevelInfo}},
		{message: inspectmessages.BadRequest, status: http.StatusBadRequest, dapp: applicationName,
			body: func() io.Reader { return failedInspectBody{} }, levels: []slog.Level{slog.LevelInfo}},
		{message: inspectmessages.TerminalApplication, status: http.StatusServiceUnavailable, dapp: applicationName,
			levels: []slog.Level{slog.LevelInfo, slog.LevelInfo},
			configure: func(machine *MockMachine, _ *MachinesMock, _ *MockRepository) {
				machine.application.Status = model.ApplicationStatus_Corrupted
			}},
		{message: inspectmessages.MachineNotReady, status: http.StatusServiceUnavailable, dapp: applicationName,
			levels:    []slog.Level{slog.LevelInfo, slog.LevelWarn},
			configure: func(_ *MockMachine, machines *MachinesMock, _ *MockRepository) { delete(machines.Map, 1) }},
		{message: inspectmessages.ForeclosedApplication, status: http.StatusServiceUnavailable, dapp: applicationName,
			levels: []slog.Level{slog.LevelInfo, slog.LevelInfo},
			configure: func(machine *MockMachine, machines *MachinesMock, _ *MockRepository) {
				machine.application.ForecloseBlock = 1
				delete(machines.Map, 1)
			}},
		{message: inspectmessages.ApplicationNotFound, status: http.StatusNotFound, dapp: applicationName,
			levels:    []slog.Level{slog.LevelInfo, slog.LevelInfo},
			configure: func(_ *MockMachine, _ *MachinesMock, repository *MockRepository) { repository.apps = nil }},
		{message: inspectmessages.InspectAtCapacity, status: http.StatusServiceUnavailable, dapp: applicationName,
			levels: []slog.Level{slog.LevelInfo, slog.LevelInfo},
			configure: func(machine *MockMachine, machines *MachinesMock, _ *MockRepository) {
				machine.inspectError = manager.ErrInspectAtCapacity
				machines.Map[1] = *machine
			}},
	} {
		t.Run(test.message, func(t *testing.T) {
			machine := newMockMachine(1)
			machine.application.Status = model.ApplicationStatus_OK
			machines := newMockMachines()
			machines.Map[1] = *machine
			repository := newMockRepository()
			repository.apps = append(repository.apps, machine.application)
			if test.configure != nil {
				test.configure(machine, machines, repository)
			}
			inspector := &Inspector{repository: repository, IInspectMachines: machines}
			var logs bytes.Buffer
			inspector.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			var calls int
			client, err := inspectclient.NewSafeClient(
				"http://operator:ENDPOINT_CREDENTIAL@example.invalid/base?key=ENDPOINT_CREDENTIAL",
				inspectclient.WithHTTPClient(&http.Client{
					Transport: inspectHandlerTransport(func(request *http.Request) (*http.Response, error) {
						calls++
						defer request.Body.Close()
						recorder := httptest.NewRecorder()
						service.RequestIDMiddleware(inspector).ServeHTTP(recorder, request)
						return recorder.Result(), nil
					}),
				}))
			require.NoError(t, err)
			var body io.Reader = strings.NewReader("payload")
			if test.body != nil {
				body = test.body()
			}
			response, err := client.InspectPostWithBody(t.Context(), test.dapp, "application/octet-stream", body,
				func(_ context.Context, request *http.Request) error {
					request.Header.Set("X-Request-ID", inspectLogRequestID)
					request.SetPathValue("dapp", test.dapp)
					if test.method != "" {
						request.Method = test.method
					}
					return nil
				})
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, 1, calls)
			require.Equal(t, test.status, response.StatusCode)
			message, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, test.message, string(message))
			require.NotContains(t, string(message), "CREDENTIAL")
			require.Equal(t, inspectLogRequestID, response.Header.Get("X-Request-ID"))
			requireInspectRequestLogs(t, &logs, test.levels)
			inspector.Logger.Info("outside the request")
			require.NotContains(t, logs.String(), "request_id")
		})
	}
}

func TestInspectorUnexpectedFailureLogsRequestIDAndPrivateCause(t *testing.T) {
	var logs bytes.Buffer
	inspector, app := newInspectorForTest(t, errors.New("SAFE_MACHINE_FAILURE"))
	inspector.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	request := httptest.NewRequest(http.MethodPost, "/inspect/"+app.Name, strings.NewReader("payload"))
	request.Header.Set("X-Request-ID", inspectLogRequestID)
	recorder := httptest.NewRecorder()
	inspector.Server.Handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Equal(t, inspectLogRequestID, recorder.Header().Get("X-Request-ID"))
	require.Contains(t, recorder.Body.String(), "Internal server error (request_id="+inspectLogRequestID+")")
	require.NotContains(t, recorder.Body.String(), "SAFE_MACHINE_FAILURE")
	require.Contains(t, logs.String(), "SAFE_MACHINE_FAILURE", "the cause must stay in the server log")
	requireInspectRequestLogs(t, &logs, []slog.Level{slog.LevelInfo, slog.LevelError})
}
