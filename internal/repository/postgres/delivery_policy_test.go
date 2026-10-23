// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"encoding/json"
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/stretchr/testify/require"
)

func TestDeliveryRegistrationSupportsSelectedNonPendingPolicy(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Latest, model.DefaultBlock_Safe, model.DefaultBlock_Finalized} {
		for _, submitting := range []bool{false, true} {
			name := policy.String() + "/observer"
			if submitting {
				name = policy.String() + "/submitting"
			}
			t.Run(name, func(t *testing.T) {
				r := newEpochPublicationRepository(t).(*PostgresRepository)
				chain := config.PersistentChainConfig{DefaultBlock: policy, ChainID: 1}
				_, err := r.RegisterObserver(t.Context(), deliveryObserverKey, chain)
				require.NoError(t, err)
				role := config.PersistentSubmitterConfig{DefaultBlock: policy, ChainID: 1, ClaimSubmissionEnabled: submitting}
				if submitting {
					role.ResolvedSignerAddress = repotest.UniqueAddress()
				}
				_, err = r.RegisterSubmitter(t.Context(), deliveryPRTKey, role)
				require.NoError(t, err)
				require.NoError(t, r.CheckSubmitterProvenance(t.Context(), deliveryPRTKey))
				// Explicit policy selection does not permit changing a saved deployment.
				role.DefaultBlock = model.DefaultBlock_Latest
				if policy == model.DefaultBlock_Latest {
					role.DefaultBlock = model.DefaultBlock_Finalized
				}
				_, err = r.RegisterSubmitter(t.Context(), deliveryPRTKey, role)
				require.ErrorContains(t, err, "observation policy mismatch")
				chain.DefaultBlock = model.DefaultBlock_Pending
				_, err = r.RegisterObserver(t.Context(), deliveryObserverKey, chain)
				require.ErrorContains(t, err, "pending")
			})
		}
	}
}

func TestDeliveryRegistrationRejectsCrossRecordPolicyMismatch(t *testing.T) {
	role := func(policy model.DefaultBlock, chain uint64) config.PersistentSubmitterConfig {
		return config.PersistentSubmitterConfig{DefaultBlock: policy, ChainID: chain,
			ClaimSubmissionEnabled: true, ResolvedSignerAddress: repotest.UniqueAddress()}
	}
	for _, first := range []string{deliveryClaimerKey, deliveryPRTKey} {
		for _, otherPolicy := range []model.DefaultBlock{model.DefaultBlock_Safe, model.DefaultBlock_Finalized} {
			t.Run(first+"/"+otherPolicy.String(), func(t *testing.T) {
				r := newEpochPublicationRepository(t).(*PostgresRepository)
				second := deliveryClaimerKey
				if first == second {
					second = deliveryPRTKey
				}
				_, err := r.RegisterSubmitter(t.Context(), first, role(model.DefaultBlock_Latest, 1))
				require.NoError(t, err)
				_, err = r.RegisterSubmitter(t.Context(), second, role(otherPolicy, 1))
				require.ErrorContains(t, err, "observation policy mismatch")
				_, _, _, err = r.LoadNodeConfigRaw(t.Context(), second)
				require.ErrorIs(t, err, repository.ErrNotFound, "rejected registration must not persist")
				_, err = r.RegisterSubmitter(t.Context(), second, role(model.DefaultBlock_Latest, 2))
				require.ErrorContains(t, err, "chain ID mismatch")
				_, err = r.RegisterSubmitter(t.Context(), second, role(model.DefaultBlock_Latest, 1))
				require.NoError(t, err)
				_, err = r.RegisterObserver(t.Context(), deliveryObserverKey,
					config.PersistentChainConfig{DefaultBlock: model.DefaultBlock_Latest, ChainID: 1})
				require.NoError(t, err, "a rejected incompatible role must not prevent compatible startup")
			})
		}
	}
	for _, readerFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "role before reader", true: "reader before role"}[readerFirst], func(t *testing.T) {
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			reader := config.PersistentChainConfig{DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1}
			if readerFirst {
				_, err := r.RegisterObserver(t.Context(), deliveryObserverKey, reader)
				require.NoError(t, err)
				_, err = r.RegisterSubmitter(t.Context(), deliveryPRTKey, role(model.DefaultBlock_Latest, 1))
				require.ErrorContains(t, err, "observation policy mismatch")
			} else {
				_, err := r.RegisterSubmitter(t.Context(), deliveryPRTKey, role(model.DefaultBlock_Latest, 1))
				require.NoError(t, err)
				_, err = r.RegisterObserver(t.Context(), deliveryObserverKey, reader)
				require.ErrorContains(t, err, "observation policy mismatch")
			}
		})
	}
}

func TestDeliveryProvenanceRejectsSavedReaderPolicyMismatch(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	tx, err := r.db.Begin(t.Context())
	require.NoError(t, err)
	defer rollbackDeliveryTx(tx)
	reader, err := json.Marshal(config.PersistentChainConfig{DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1})
	require.NoError(t, err)
	role, err := json.Marshal(config.PersistentSubmitterConfig{DefaultBlock: model.DefaultBlock_Latest, ChainID: 1,
		ClaimSubmissionEnabled: true, ResolvedSignerAddress: repotest.UniqueAddress()})
	require.NoError(t, err)
	require.NoError(t, insertDeliveryConfig(t.Context(), tx, deliveryObserverKey, reader))
	require.NoError(t, insertDeliveryConfig(t.Context(), tx, deliveryPRTKey, role))
	require.NoError(t, tx.Commit(t.Context()))
	err = r.CheckSubmitterProvenance(t.Context(), deliveryPRTKey)
	require.ErrorIs(t, err, repository.ErrSubmitterProvenance)
	require.ErrorContains(t, err, "observation policy mismatch")
}

func TestDeliveryObserverOnlyRolesStillRequireOnePolicy(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	_, err := r.RegisterSubmitter(t.Context(), deliveryClaimerKey,
		config.PersistentSubmitterConfig{DefaultBlock: model.DefaultBlock_Latest, ChainID: 1})
	require.NoError(t, err)
	_, err = r.RegisterSubmitter(t.Context(), deliveryPRTKey,
		config.PersistentSubmitterConfig{DefaultBlock: model.DefaultBlock_Safe, ChainID: 1})
	require.ErrorContains(t, err, "observation policy mismatch", "shared domain facts need one policy even without publication")
}
