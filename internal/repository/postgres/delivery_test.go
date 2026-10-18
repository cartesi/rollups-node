// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func registerDeliveryTest(t *testing.T, r *PostgresRepository, signer common.Address) {
	t.Helper()
	_, err := r.RegisterObserver(t.Context(), "evm-reader", config.PersistentChainConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1,
	})
	require.NoError(t, err)
	_, err = r.RegisterSubmitter(t.Context(), "prt", config.PersistentSubmitterConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1, ClaimSubmissionEnabled: true, ResolvedSignerAddress: signer,
	})
	require.NoError(t, err)
}

func TestDeliveryConcurrentRegistrationAndProvenance(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	signer := repotest.UniqueAddress()
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, key := range []string{"claimer", "prt"} {
		go func() {
			<-start
			_, err := r.RegisterSubmitter(t.Context(), key, config.PersistentSubmitterConfig{
				DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1, ClaimSubmissionEnabled: true, ResolvedSignerAddress: signer,
			})
			results <- err
		}()
	}
	close(start)
	a, b := <-results, <-results
	require.True(t, (a == nil) != (b == nil), "only one role may register a shared signer: %v / %v", a, b)
	key := "claimer"
	var exists bool
	require.NoError(t, r.db.QueryRow(t.Context(), "SELECT EXISTS(SELECT 1 FROM node_config WHERE key='prt')").Scan(&exists))
	if exists {
		key = "prt"
	}
	require.ErrorIs(t, r.CheckSubmitterProvenance(t.Context(), key), repository.ErrObserverNotRegistered)
	_, err := r.RegisterObserver(t.Context(), "evm-reader", config.PersistentChainConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1,
	})
	require.NoError(t, err)
	require.NoError(t, r.CheckSubmitterProvenance(t.Context(), key))
	_, err = r.RegisterObserver(t.Context(), "evm-reader", config.PersistentChainConfig{
		DefaultBlock: model.DefaultBlock_Pending, ChainID: 1,
	})
	require.Error(t, err)
}

func TestDeliveryCannotInventObserverProvenance(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	epoch := repotest.NewEpochBuilder(app.ID).WithStatus(model.EpochStatus_Closed).Build()
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, map[*model.Epoch][]*model.Input{epoch: {}}, 10))
	_, err := r.RegisterObserver(t.Context(), "evm-reader", config.PersistentChainConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1,
	})
	require.ErrorContains(t, err, "provenance is absent")
	_, _, _, err = r.LoadNodeConfigRaw(t.Context(), "evm-reader")
	require.ErrorIs(t, err, repository.ErrNotFound)
}

func TestDeliveryApplicationKeysetIgnoresRenames(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	apps := make([]*model.Application, 0, 3)
	for _, name := range []string{"z", "m", "a"} {
		apps = append(apps, repotest.NewApplicationBuilder().WithName(name).Create(t.Context(), t, r))
	}
	boundary, err := r.GetApplicationIDBoundary(t.Context(), repository.ApplicationFilter{})
	require.NoError(t, err)
	page, err := r.ListApplicationsByID(t.Context(), repository.ApplicationFilter{}, 0, boundary, 1)
	require.NoError(t, err)
	require.Equal(t, apps[0].ID, page[0].ID)
	apps[1].Name = "zz"
	require.NoError(t, r.UpdateApplication(t.Context(), apps[1]))
	page, err = r.ListApplicationsByID(t.Context(), repository.ApplicationFilter{}, apps[0].ID, boundary, 1)
	require.NoError(t, err)
	require.Equal(t, apps[1].ID, page[0].ID)
}

func TestObserverApplicationRemovalNeedsNoSubmissionRegistration(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	_, err := r.RegisterObserver(t.Context(), "evm-reader", config.PersistentChainConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1,
	})
	require.NoError(t, err)
	_, err = r.RegisterSubmitter(t.Context(), "prt", config.PersistentSubmitterConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1,
	})
	require.NoError(t, err)
	app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).WithEnabled(false).Create(t.Context(), t, r)
	require.NoError(t, r.DeleteApplication(t.Context(), app.ID))
}
