// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/claimer"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/prt"
	"github.com/cartesi/rollups-node/internal/repository/factory"
)

// startReaderNode uses matching saved/requested settings in the isolated test
// database. This is fixture setup, not a supported production mode transition.
// Cleanup stops the reader, restores the exact saved records, and restarts the
// default node before the next test. Latest supports fixtures that pause mining.
func startReaderNode(ctx context.Context, t *testing.T) {
	t.Helper()
	dsn, err := config.GetDatabaseConnection()
	require.NoError(t, err)
	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	require.NoError(t, err)
	t.Cleanup(repo.Close)
	overrides := []struct {
		key    string
		fields map[string]any
	}{
		{prt.PrtConfigKey, map[string]any{passiveDefaultBlockField: model.DefaultBlock_Latest, passiveClaimSubmissionField: false}},
		{evmreader.EvmReaderConfigKey, map[string]any{passiveDefaultBlockField: model.DefaultBlock_Latest}},
		{claimer.ClaimerConfigKey, map[string]any{passiveDefaultBlockField: model.DefaultBlock_Latest, passiveClaimSubmissionField: false}},
	}
	type savedConfig struct {
		key          string
		raw, updated []byte
	}
	saved := make([]savedConfig, 0, len(overrides))
	registerPassiveObserverCleanup(t, func() {
		if sharedNode != nil {
			stopSharedNode(t)
		}
	}, func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		restored := true
		for _, original := range saved {
			if err := repo.SaveNodeConfigRaw(cleanupCtx, original.key, original.raw); err != nil {
				t.Errorf("restore %s persistent config: %v", original.key, err)
				restored = false
			}
		}
		require.True(t, restored, "do not restart the default node with a partially restored config")
	}, func() { startSharedNode(t) })
	// Register cleanup before stopping: an already-exited child can fail the
	// stop assertion after clearing sharedNode, but still needs a default restart.
	stopSharedNode(t)
	for _, override := range overrides {
		raw, _, _, err := repo.LoadNodeConfigRaw(ctx, override.key)
		require.NoError(t, err, "the healthy test-managed node must have initialized %s config", override.key)
		updated, err := patchPassiveObserverConfig(raw, override.fields)
		require.NoError(t, err)
		saved = append(saved, savedConfig{override.key, append([]byte(nil), raw...), updated})
	}
	// Validate all originals before the first write. Cleanup is already registered
	// even if a write succeeds but its reply fails. Never change the parent env.
	for _, override := range saved {
		require.NoError(t, repo.SaveNodeConfigRaw(ctx, override.key, override.updated))
	}
	startSharedNodeWithEnv(t, "CARTESI_FEATURE_CLAIM_SUBMISSION_ENABLED=false",
		"CARTESI_BLOCKCHAIN_DEFAULT_BLOCK=latest")
}
