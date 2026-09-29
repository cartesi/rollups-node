// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"fmt"
	"regexp"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	jsonrpcclient "github.com/cartesi/rollups-node/pkg/jsonrpc/client"
	"github.com/ethereum/go-ethereum/common"
)

// The public API does not expose the execution hash collection. Check its
// persisted rows directly while the proof and completion are checked via API.
func (s *TerminalMachineStatesSuite) terminalStateHashes(input *model.Input) []*model.StateHash {
	s.T().Helper()
	r := s.Require()
	dsn, err := config.GetDatabaseConnection()
	r.NoError(err)
	repo, err := factory.NewRepositoryFromConnectionString(s.ctx, dsn.Raw())
	r.NoError(err)
	defer repo.Close()
	rows, _, err := repo.ListStateHashes(s.ctx, s.appName,
		repository.StateHashFilter{EpochIndex: &input.EpochIndex}, repository.Pagination{}, false)
	r.NoError(err)
	r.NotEmpty(rows)
	var span uint64
	for _, row := range rows {
		r.Equal(input.Index, row.InputIndex)
		span += row.Repetitions
	}
	r.Equal(model.InputHashCollectionCapacity, span)
	r.NotNil(input.MachineHash)
	r.Equal(*input.MachineHash, rows[len(rows)-1].MachineHash)
	return rows
}

func (s *TerminalMachineStatesSuite) TestInvalidOutputsRootLengthSurvivesRestart() {
	s.runInvalidOutputsRootLength(false)
}

func (s *TerminalMachineStatesSuite) TestInvalidOutputsRootLengthPrtSurvivesRestart() {
	s.runInvalidOutputsRootLength(true)
}

func (s *TerminalMachineStatesSuite) runInvalidOutputsRootLength(prt bool) {
	s.T().Helper()
	tc := terminalMachineStateCase{
		namePrefix:        "invalid-root-length",
		dappPathEnv:       "CARTESI_TEST_INVALID_OUTPUTS_ROOT_LENGTH_DAPP_PATH",
		defaultDappPath:   "applications/invalid-outputs-root-length-dapp",
		payloadPrefix:     "invalid-root-length",
		description:       "a guest that declares a 31-byte root in an accepted yield",
		inputStatus:       model.InputCompletionStatus_InvalidOutputsRoot,
		applicationStatus: model.ApplicationStatus_InvalidOutputsRoot,
		reasonSuffix:      ": an accepted yield must declare exactly 32 bytes",
	}
	if prt {
		tc.deployArgs = []string{prtFlag}
	}
	s.runTerminalMachineState(tc)
}

func (s *TerminalMachineStatesSuite) TestInvalidOutputsRootValue() {
	s.appName = uniqueAppName("invalid-root-value")
	dappPath := envOrDefault("CARTESI_TEST_INVALID_OUTPUTS_ROOT_DAPP_PATH", "applications/invalid-outputs-root-dapp")
	_, err := deployApplication(s.ctx, s.appName, dappPath, terminalFixtureSaltFlag, uniqueSalt())
	s.Require().NoError(err)
	index, _, err := sendInput(s.ctx, s.appName, "invalid-root-value")
	s.Require().NoError(err)
	s.Require().Zero(index)
	ctx, cancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	defer cancel()
	input, err := waitForInputProcessed(ctx, s.T(), s.appName, index)
	s.Require().NoError(err)
	s.Require().Equal(model.InputCompletionStatus_Accepted, input.Status,
		"a later validator finding must not rewrite the completed input")
	// Root values are checked per epoch, not when the input completes. Drive
	// the Authority epoch past its last block before waiting for validation.
	minePastEpochBoundary(ctx, s.T(), s.Require(), s.appName, input.EpochIndex)
	s.requireInvalidOutputsRoot(ctx, input.EpochIndex, 1)
	stored, err := readInput(s.ctx, s.appName, index)
	s.Require().NoError(err)
	s.Require().Equal(model.InputCompletionStatus_Accepted, stored.Status)
}

func (s *TerminalMachineStatesSuite) TestInvalidTemplateOutputsRootInFirstPrtEpoch() {
	s.appName = uniqueAppName("invalid-template-root")
	dappPath := envOrDefault("CARTESI_TEST_INVALID_TEMPLATE_OUTPUTS_ROOT_DAPP_PATH",
		"applications/invalid-template-outputs-root-dapp")
	_, err := deployApplication(s.ctx, s.appName, dappPath, terminalFixtureSaltFlag, uniqueSalt(), prtFlag)
	s.Require().NoError(err)
	ctx, cancel := context.WithTimeout(s.ctx, inputProcessingTimeout)
	defer cancel()
	s.requireInvalidOutputsRoot(ctx, 0, 0)
	epoch, err := readEpoch(s.ctx, s.appName, 0)
	s.Require().NoError(err)
	s.Require().Zero(epoch.InputIndexLowerBound)
	s.Require().Zero(epoch.InputIndexUpperBound, "the first sealed epoch must contain no inputs")
}

func (s *TerminalMachineStatesSuite) requireInvalidOutputsRoot(ctx context.Context, epochIndex, processed uint64) {
	s.T().Helper()
	r := s.Require()
	r.NoError(waitForApplicationStatus(ctx, s.T(), s.appName, string(model.ApplicationStatus_InvalidOutputsRoot)))
	rpc := jsonrpcclient.NewClient(envOrDefault("CARTESI_JSONRPC_API_URL", "http://localhost:10011/rpc"))
	app := s.getApplication(rpc)
	r.Equal(model.ApplicationStatus_InvalidOutputsRoot, app.Status)
	r.Equal(processed, app.ProcessedInputs)
	reason := fmt.Sprintf(
		"epoch %d: declared outputs root does not match the root calculated from stored outputs; declared=%s; calculated=%s",
		epochIndex, common.Hash{}.Hex(), merkle.CreatePostContext()[merkle.TREE_DEPTH].Hex())
	r.Equal(&reason, app.Reason)
	s.SetExpectedLogs(s.T(), ExpectedLog{
		Pattern: regexp.MustCompile(`marking application with invalid outputs root \(terminal\).*application=` +
			regexp.QuoteMeta(s.appName)),
		Level: LevelError, Required: true,
		Reason: "the validator records the tested application's failed root rule",
	}, ExpectedLog{
		Pattern: regexp.MustCompile(`Tick service=validator.*` + regexp.QuoteMeta(reason)),
		Level:   LevelError, Required: true,
		Reason: "the validator returns the exact root-rule failure to its service loop",
	})
	epoch, err := readEpoch(s.ctx, s.appName, epochIndex)
	r.NoError(err)
	r.Equal(model.EpochStatus_InputsProcessed, epoch.Status)
	r.Nil(epoch.ClaimTransactionHash, "the failing epoch must not publish a claim")
	r.Nil(epoch.Commitment, "the failing PRT epoch must not publish a commitment")
	r.True(epoch.HasCompleteStateProof())

	// Enabling observation is not a request to clear an immutable diagnosis.
	r.NoError(disableApplication(s.ctx, s.appName))
	_, err = runCLI(s.ctx, "app", "status", s.appName, "enabled", "--yes")
	r.NoError(err)
	app = s.getApplication(rpc)
	r.True(app.Enabled)
	r.Equal(model.ApplicationStatus_InvalidOutputsRoot, app.Status)
	r.Equal(&reason, app.Reason)
	status, err := readApplicationStatus(s.ctx, s.appName)
	r.NoError(err)
	r.Equal(string(model.ApplicationStatus_InvalidOutputsRoot), firstStatusLine(status))
	r.Contains(status, reason)
}
