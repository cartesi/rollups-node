// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"context"
	"testing"
	"time"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const testTimeout = 300 * time.Second

// RepositoryFactory creates a fresh repository backed by an empty schema.
// Called once per sub-test for full isolation.
type RepositoryFactory func(ctx context.Context, t *testing.T) (
	repo repository.Repository,
	cleanup func(),
)

// BaseSuite is embedded by every per-interface suite.
// SetupSubTest calls the factory to get a fresh repo per s.Run() block.
// TearDownSubTest calls cleanup.
type BaseSuite struct {
	suite.Suite
	factory RepositoryFactory
	Repo    repository.Repository
	cleanup func()
	Ctx     context.Context
	cancel  context.CancelFunc
}

// StoreAdvanceResult is a test helper that creates outputs and/or reports
// for an input via the production StoreAdvanceResult method.
func StoreAdvanceResult(
	ctx context.Context, t *testing.T, repo repository.Repository,
	appID int64, epochIdx, inputIdx uint64,
	status InputCompletionStatus, outputs [][]byte, reports [][]byte,
) {
	t.Helper()
	result := &AdvanceResult{
		EpochIndex: epochIdx,
		InputIndex: inputIdx,
		Status:     status,
		Outputs:    outputs,
		Reports:    reports,
		OutputsProof: OutputsProof{
			OutputsHash: UniqueHash(),
			MachineHash: UniqueHash(),
		},
	}
	err := repo.StoreAdvanceResult(ctx, appID, result)
	require.NoError(t, err)
}

// storeAdvanceResult delegates to the standalone StoreAdvanceResult helper
// with InputCompletionStatus_Accepted as the default status.
func (s *BaseSuite) storeAdvanceResult(
	appID int64, epochIdx, inputIdx uint64, outputs [][]byte, reports [][]byte,
) {
	s.T().Helper()
	StoreAdvanceResult(s.Ctx, s.T(), s.Repo, appID, epochIdx, inputIdx,
		InputCompletionStatus_Accepted, outputs, reports)
}

func (s *BaseSuite) SetupSuite() {
	s.Ctx, s.cancel = context.WithTimeout(context.Background(), testTimeout)
}

func (s *BaseSuite) TearDownSuite() {
	s.cancel()
}

func (s *BaseSuite) SetupSubTest() {
	s.Repo, s.cleanup = s.factory(s.Ctx, s.T())
}

func (s *BaseSuite) TearDownSubTest() {
	if s.cleanup != nil {
		s.cleanup()
	}
}

// RunAllSuites is the single entry point backends call.
func RunAllSuites(t *testing.T, factory RepositoryFactory) {
	t.Run("Application", func(t *testing.T) { suite.Run(t, NewApplicationSuite(factory)) })
	t.Run("Epoch", func(t *testing.T) { suite.Run(t, NewEpochSuite(factory)) })
	t.Run("Input", func(t *testing.T) { suite.Run(t, NewInputSuite(factory)) })
	t.Run("Output", func(t *testing.T) { suite.Run(t, NewOutputSuite(factory)) })
	t.Run("Report", func(t *testing.T) { suite.Run(t, NewReportSuite(factory)) })
	t.Run("StateHash", func(t *testing.T) { suite.Run(t, NewStateHashSuite(factory)) })
	t.Run("BulkOperations", func(t *testing.T) { suite.Run(t, NewBulkOperationsSuite(factory)) })
	t.Run("NodeConfig", func(t *testing.T) { suite.Run(t, NewNodeConfigSuite(factory)) })
	t.Run("Claimer", func(t *testing.T) { suite.Run(t, NewClaimerSuite(factory)) })
	t.Run("Tournament", func(t *testing.T) { suite.Run(t, NewTournamentSuite(factory)) })
	t.Run("Commitment", func(t *testing.T) { suite.Run(t, NewCommitmentSuite(factory)) })
	t.Run("Match", func(t *testing.T) { suite.Run(t, NewMatchSuite(factory)) })
	t.Run("MatchAdvanced", func(t *testing.T) { suite.Run(t, NewMatchAdvancedSuite(factory)) })
	t.Run("Withdrawal", func(t *testing.T) { suite.Run(t, NewWithdrawalSuite(factory)) })
}
