// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicationAndCancellationPreserveHumanActionContext(t *testing.T) {
	s, _, _ := testSender(t)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	p := newPlanner()
	p.request.Logger = logger.With("application", "example", "application_address", "0xapp", "epoch", 7,
		"action", "join tournament")
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	p.decision, p.eligibility = cancelDecision, Obsolete
	_, err = s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	decoder := json.NewDecoder(&logs)
	var last map[string]any
	for decoder.More() {
		require.NoError(t, decoder.Decode(&last))
	}
	require.Equal(t, "example", last["application"])
	require.Equal(t, "0xapp", last["application_address"])
	require.Equal(t, "join tournament", last["action"])
	require.EqualValues(t, 7, last["epoch"])
	require.Equal(t, true, last["neutral"])
	require.NotContains(t, logs.String(), "Identity")
	require.NotContains(t, logs.String(), "action_key")
}
