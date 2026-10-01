// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/outputs"
)

// The empty-tree root of the height-63 outputs tree, as written by libcmt.
var emptyOutputsRoot = common.HexToHash("0x0a162946e56158bac0673e6dd3bdfdc1e4a0e7744a120fdb640050c8d7abe1c6")

func TestOutputsMerkleRootEmpty(t *testing.T) {
	require.Equal(t, emptyOutputsRoot, outputsMerkleRoot(nil))
}

func TestOutputsMerkleRootMatchesNodeMerklePackage(t *testing.T) {
	outputs := [][]byte{[]byte("voucher"), []byte("notice"), {}, []byte("third")}
	for n := 1; n <= len(outputs); n++ {
		leaves := make([]common.Hash, n)
		for i := range n {
			leaves[i] = crypto.Keccak256Hash(outputs[i])
		}
		want, _, err := merkle.CreateProofs(leaves, outputsTreeHeight)
		require.NoError(t, err)
		require.Equal(t, want, outputsMerkleRoot(outputs[:n]), "outputs %d", n)
	}
}

func TestParseSlingClaims(t *testing.T) {
	log := strings.Join([]string{
		"[2026-09-25T03:04:49Z INFO  cartesi_rollups_prt_node::epoch_manager] stage tournament result of epoch 0 with claim " +
			"0x3c020b49b4948e7b032c1267e23da046dcc10172550b52d5d4d71ebb6e2b3924",
		"unrelated line",
		"[x] stage tournament result of epoch 2 with claim 0x" + strings.Repeat("ab", 32),
		// A retry logs the same claim again.
		"[y] stage tournament result of epoch 0 with claim 0x3c020b49b4948e7b032c1267e23da046dcc10172550b52d5d4d71ebb6e2b3924",
	}, "\n")
	claims, err := parseSlingClaims(strings.NewReader(log))
	require.NoError(t, err)
	require.Equal(t, []epochClaim{
		{Epoch: 0, Commitment: common.HexToHash("0x3c020b49b4948e7b032c1267e23da046dcc10172550b52d5d4d71ebb6e2b3924")},
		{Epoch: 2, Commitment: common.HexToHash("0x" + strings.Repeat("ab", 32))},
	}, claims)
}

func TestParseSlingClaimsRejectsConflicts(t *testing.T) {
	log := "stage tournament result of epoch 1 with claim 0x" + strings.Repeat("01", 32) + "\n" +
		"stage tournament result of epoch 1 with claim 0x" + strings.Repeat("02", 32) + "\n"
	_, err := parseSlingClaims(strings.NewReader(log))
	require.ErrorContains(t, err, "two claims for epoch 1")
}

func validManifest() *caseManifest {
	return &caseManifest{
		Schema: manifestSchemaVersion, Case: "honeypot-stf_all", CreatedAt: time.Now(), Golden: true,
		Chain:       caseChain{ChainID: anvilChainID, Head: 10},
		Application: caseApplication{Address: common.HexToAddress("0x01"), Consensus: common.HexToAddress("0x02"), TemplateDir: "template"},
		Deployments: caseDeployments{InputBox: common.HexToAddress("0x03")},
		Artifacts:   caseArtifacts{Dump: caseFile{Path: "anvil-state.json.zst", SHA256: "aa"}, DaveLog: "dave.log"},
	}
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := validManifest()
	m.Reference.Claims = []epochClaim{{Epoch: 1, Commitment: common.HexToHash("0x05")}}
	require.NoError(t, writeManifest(dir, m))
	read, err := readManifest(dir)
	require.NoError(t, err)
	require.Equal(t, m.Reference.claimMap(), read.Reference.claimMap())
	require.Equal(t, m.Application, read.Application)
}

func TestManifestValidation(t *testing.T) {
	for name, mutate := range map[string]func(*caseManifest){
		"escaping path":           func(m *caseManifest) { m.Application.TemplateDir = "../elsewhere" },
		"absolute path":           func(m *caseManifest) { m.Artifacts.Dump.Path = "/tmp/dump" },
		"golden but failed":       func(m *caseManifest) { m.Provenance.HarnessExitCode = 1 },
		"golden but not observed": func(m *caseManifest) { m.Provenance.ExitCodeSource = exitCodeOperator },
		"duplicate claim": func(m *caseManifest) {
			m.Reference.Claims = []epochClaim{{Epoch: 1}, {Epoch: 1}}
		},
		"no checksum":    func(m *caseManifest) { m.Artifacts.Dump.SHA256 = "" },
		"wrong schema":   func(m *caseManifest) { m.Schema = 99 },
		"no application": func(m *caseManifest) { m.Application.Address = common.Address{} },
	} {
		t.Run(name, func(t *testing.T) {
			m := validManifest()
			mutate(m)
			require.Error(t, m.validate())
		})
	}
	require.NoError(t, validManifest().validate())
}

func TestManifestRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeManifest(dir, validManifest()))
	path := filepath.Join(dir, manifestFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), "{", `{"surprise": 1,`, 1)), 0o600)) //nolint:gosec
	_, err = readManifest(dir)
	require.ErrorContains(t, err, "surprise")
}

func TestZstdRoundTrip(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "raw.json")
	data := []byte(strings.Repeat(`{"state":"0x00"},`, 10000))
	require.NoError(t, os.WriteFile(raw, data, 0o600))
	require.NoError(t, compressZstd(raw, raw+".zst"))
	require.NoError(t, decompressZstd(raw+".zst", filepath.Join(dir, "out.json")))
	out, err := os.ReadFile(filepath.Join(dir, "out.json"))
	require.NoError(t, err)
	require.Equal(t, data, out)
	// Neither function overwrites an existing file.
	require.Error(t, compressZstd(raw, raw+".zst"))
}

func TestMergeEnvironment(t *testing.T) {
	const fallback = "default"
	env := mergeEnvironment(
		[]string{"A=1", "B=", "C=keep", "DB=operator"},
		map[string]string{"A": fallback, "B": fallback, "E": fallback},
		map[string]string{"DB": "run", "F": "run"},
	)
	require.Equal(t, "1", lookupEnv(env, "A"), "defaults never replace a set value")
	require.Equal(t, fallback, lookupEnv(env, "B"), "defaults fill empty values")
	require.Equal(t, "keep", lookupEnv(env, "C"))
	require.Equal(t, "run", lookupEnv(env, "DB"), "overrides always win")
	require.Equal(t, fallback, lookupEnv(env, "E"))
	require.Equal(t, "run", lookupEnv(env, "F"))
}

func TestDeriveTestAccount(t *testing.T) {
	account, err := deriveTestAccount(testMnemonic, defaultWithdrawerIx)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress(defaultHoneypotWithdrawer), account.Address)
	require.Error(t, requireTestChain(1))
	require.NoError(t, requireTestChain(anvilChainID))
}

func TestChildReportsEarlyExit(t *testing.T) {
	dir := t.TempDir()
	c, err := startChild("exit7", filepath.Join(dir, "log"), dir, os.Environ(), "sh", "-c", "exit 7")
	require.NoError(t, err)
	<-c.done
	ok, code, _ := c.exited()
	require.True(t, ok)
	require.Equal(t, 7, code)
	require.ErrorContains(t, c.errIfExited(), "exited early")

	zero, err := startChild("exit0", filepath.Join(dir, "log0"), dir, os.Environ(), "sh", "-c", "exit 0")
	require.NoError(t, err)
	<-zero.done
	require.ErrorContains(t, zero.errIfExited(), "exited early", "a zero exit of a long-running child is still early")
}

func TestChildStopReachesProcessGroup(t *testing.T) {
	dir := t.TempDir()
	c, err := startChild("sleeper", filepath.Join(dir, "log"), dir, os.Environ(), "sh", "-c", "sleep 60 & wait")
	require.NoError(t, err)
	c.stop()
	ok, _, _ := c.exited()
	require.True(t, ok)
}

// A child that exits at once can leave its own children in the group; stop
// must end them too.
func TestChildStopKillsLeftoverGrandchildren(t *testing.T) {
	dir := t.TempDir()
	c, err := startChild("leader", filepath.Join(dir, "log"), dir, os.Environ(), "sh", "-c", "sleep 60 & exit 0")
	require.NoError(t, err)
	<-c.done
	pgid := c.cmd.Process.Pid
	require.True(t, groupAlive(pgid), "the grandchild outlives the leader")
	c.stop()
	require.False(t, groupAlive(pgid))
	c.stop() // a second stop is harmless
}

func TestRunDatabaseName(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	require.Equal(t, "interop_honeypot_stf_all_20260929t120000", runDatabaseName("honeypot-stf_all", now))
	long := runDatabaseName(strings.Repeat("very-long-case-name-", 5), now)
	require.LessOrEqual(t, len(long), 63)
	require.True(t, strings.HasPrefix(long, interopDatabasePrefix), long)
	require.True(t, strings.HasSuffix(long, "_20260929t120000"), long)
	require.Regexp(t, databaseNamePattern, long)
}

func TestSelectDaveApp(t *testing.T) {
	one := daveApp{App: common.HexToAddress("0x0a")}
	two := daveApp{App: common.HexToAddress("0x0b")}
	app, err := selectDaveApp([]daveApp{one}, "")
	require.NoError(t, err)
	require.Equal(t, one, app)
	_, err = selectDaveApp([]daveApp{one, two}, "")
	require.ErrorContains(t, err, "select one with --app")
	_, err = selectDaveApp([]daveApp{one, two}, strings.ToUpper(two.App.Hex()[2:]))
	require.Error(t, err, "a non-0x address is not accepted as a match")
	app, err = selectDaveApp([]daveApp{one, two}, two.App.Hex())
	require.NoError(t, err)
	require.Equal(t, two, app)
	_, err = selectDaveApp(nil, "")
	require.Error(t, err)
}

// --- verifier ---

func hashPtr(value string) *common.Hash {
	h := common.HexToHash(value)
	return &h
}

func addressPtr(value string) *common.Address {
	a := common.HexToAddress(value)
	return &a
}

var (
	sampleApp       = common.HexToAddress("0x01")
	sampleConsensus = common.HexToAddress("0x02")
	sampleTemplate  = common.HexToHash("0x03")
	sampleRoot0     = common.HexToAddress("0xa0")
	sampleRoot1     = common.HexToAddress("0xa1")
	sampleInner     = common.HexToAddress("0xb0")
)

// sampleFacts returns a consistent snapshot: epoch 0 accepted (state changed)
// with inputs 0 and 1, epoch 1 sealed and computed, epoch 2 open, one inner
// tournament with one match.
func sampleFacts() *verifyFacts {
	join := commitmentJoin{Commitment: common.HexToHash("0xc0"), FinalState: common.HexToHash("0xf0"), Block: 5,
		Tx: common.HexToHash("0x55")}
	return &verifyFacts{
		chainFacts: chainFacts{
			Head: 100, ChainConsensus: sampleConsensus, ChainTemplate: sampleTemplate, ChainInputCount: 2,
			ChainInputs: []chainInput{
				{Index: 0, Raw: []byte("zero"), Block: 3, Tx: common.HexToHash("0x30"), LogIndex: 1},
				{Index: 1, Raw: []byte("one"), Block: 4, Tx: common.HexToHash("0x40")},
			},
			Sealed: []sealedEpoch{
				{Index: 0, Lower: 0, Upper: 2, InitialState: common.HexToHash("0x11"), Tournament: sampleRoot0, Block: 10},
				{Index: 1, Lower: 2, Upper: 2, InitialState: common.HexToHash("0xf0"), Tournament: sampleRoot1, Block: 20,
					Tx: common.HexToHash("0x20")},
			},
			Staged: map[uint64]stagedEpoch{
				0: {Index: 0, FinalState: common.HexToHash("0xf0"), OutputsRoot: common.HexToHash("0xe0"), Block: 15},
			},
			ChainTournaments: map[common.Address]chainTournament{sampleRoot0: {}, sampleRoot1: {Epoch: 1},
				sampleInner: {Parent: sampleRoot0}},
			ChainJoins: map[common.Address][]commitmentJoin{sampleRoot0: {join}},
			ChainMatches: []chainMatch{{Tournament: sampleInner, ID: common.HexToHash("0x99"), One: common.HexToHash("0xc0"),
				Block: 7, Tx: common.HexToHash("0x77"), Deleted: true, DeleteTx: common.HexToHash("0x88")}},
			Standings:   map[uint64]rootStanding{0: {Finished: true, HasWinner: true, Winner: common.HexToHash("0xc0")}, 1: {}},
			SlingJoins:  map[uint64]commitmentJoin{0: join},
			SlingSentry: map[uint64]common.Hash{0: common.HexToHash("0xf0"), 1: common.HexToHash("0xf0")},
		},
		nodeFacts: nodeFacts{
			App: &model.Application{Status: model.ApplicationStatus_OK, IApplicationAddress: sampleApp,
				IConsensusAddress: sampleConsensus, TemplateHash: sampleTemplate, ConsensusType: model.Consensus_PRT,
				LastInputCheckBlock: 100, LastEpochCheckBlock: 100, LastTournamentCheckBlock: 100},
			Inputs: []model.Input{
				{Index: 0, RawData: []byte("zero"), BlockNumber: 3, TransactionHash: common.HexToHash("0x30"), LogIndex: 1,
					Status: model.InputCompletionStatus_Accepted},
				{Index: 1, RawData: []byte("one"), BlockNumber: 4, TransactionHash: common.HexToHash("0x40"),
					Status: model.InputCompletionStatus_Rejected},
			},
			Epochs: map[uint64]model.Epoch{
				0: {Index: 0, InputIndexLowerBound: 0, InputIndexUpperBound: 2, LastBlock: 10, Status: model.EpochStatus_ClaimAccepted,
					TournamentAddress: addressPtr("0xa0"), Commitment: hashPtr("0xc0"), MachineHash: hashPtr("0xf0"),
					TxBufferDataBlock: hashPtr("0xe0"), ClaimTransactionHash: hashPtr("0x20")},
				1: {Index: 1, InputIndexLowerBound: 2, InputIndexUpperBound: 2, LastBlock: 20, Status: model.EpochStatus_ClaimComputed,
					TournamentAddress: addressPtr("0xa1"), Commitment: hashPtr("0xc1"), MachineHash: hashPtr("0xf0"),
					TxBufferDataBlock: hashPtr("0xe0")},
				2: {Index: 2, InputIndexLowerBound: 2, InputIndexUpperBound: 2, Status: model.EpochStatus_Open},
			},
			Tournaments: map[common.Address]model.Tournament{sampleRoot0: {Address: sampleRoot0},
				sampleRoot1: {Address: sampleRoot1, EpochIndex: 1},
				sampleInner: {Address: sampleInner, ParentTournamentAddress: addressPtr("0xa0")}},
			Commitments: []model.Commitment{{TournamentAddress: sampleRoot0, Commitment: join.Commitment,
				FinalStateHash: join.FinalState, BlockNumber: 5, TxHash: join.Tx}},
			Matches: []model.Match{{TournamentAddress: sampleInner, IDHash: common.HexToHash("0x99"),
				CommitmentOne: common.HexToHash("0xc0"), BlockNumber: 7, TxHash: common.HexToHash("0x77"),
				DeletionReason: model.MatchDeletionReason_STEP, DeletionTxHash: hashPtr("0x88")}},
		},
	}
}

func sampleClaims() map[uint64]common.Hash {
	return map[uint64]common.Hash{0: common.HexToHash("0xc0"), 1: common.HexToHash("0xc1")}
}

func sampleTarget(claims map[uint64]common.Hash, requireAgreement bool) *verifyTarget {
	return &verifyTarget{appAddress: sampleApp, consensus: sampleConsensus, claims: claims,
		expectedStatus: model.ApplicationStatus_OK, requireAgreement: requireAgreement}
}

func coverageOf(report *verifyReport, name string) coverageItem {
	for _, item := range report.Coverage {
		if item.Name == name {
			return item
		}
	}
	return coverageItem{}
}

func statusOf(checks []checkResult, name string) checkStatus {
	for _, check := range checks {
		if check.Name == name {
			return check.Status
		}
	}
	return ""
}

func TestEvaluateConsistentFactsPass(t *testing.T) {
	f := sampleFacts()
	gaps, terminal := completionGaps(f, model.ApplicationStatus_OK)
	require.Empty(t, gaps)
	require.False(t, terminal)
	report := &verifyReport{Checks: evaluate(f, sampleTarget(sampleClaims(), true))}
	summarize(report)
	for _, check := range report.Checks {
		require.Equal(t, checkPass, check.Status, "%s: %s %v", check.Name, check.Detail, check.Items)
	}
	require.True(t, report.Passed)
	require.True(t, coverageOf(report, coverageAgreement).Tested)
	require.True(t, coverageOf(report, coverageTournaments).Tested)
}

func mutateEpoch(f *verifyFacts, index uint64, mutate func(*model.Epoch)) {
	epoch := f.Epochs[index]
	mutate(&epoch)
	f.Epochs[index] = epoch
}

func TestEvaluateDetectsMismatches(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*verifyFacts, map[uint64]common.Hash)
		check  string
	}{
		"wrong Sling claim": {func(_ *verifyFacts, c map[uint64]common.Hash) { c[1] = common.HexToHash("0xdead") },
			checkNameSlingCommitment},
		"wrong root winner": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			f.Standings[0] = rootStanding{Finished: true, HasWinner: true, Winner: common.HexToHash("0xbad")}
		}, "commitments.root_winner"},
		"wrong staged outputs root": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			f.Staged[0] = stagedEpoch{FinalState: common.HexToHash("0xf0"), OutputsRoot: common.HexToHash("0x00")}
		}, checkNameStaged},
		"wrong staged block": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			mutateEpoch(f, 0, func(e *model.Epoch) { block := uint64(16); e.StagedAtBlock = &block })
		}, checkNameStaged},
		"staged in the node only": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			mutateEpoch(f, 1, func(e *model.Epoch) { e.Status = model.EpochStatus_ClaimStaged })
		}, checkNameStaged},
		"wrong bounds": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			mutateEpoch(f, 1, func(e *model.Epoch) { e.InputIndexUpperBound = 3 })
		}, checkNameSealed},
		"no sealed epochs": {func(f *verifyFacts, _ map[uint64]common.Hash) { f.Sealed = nil }, checkNameSealed},
		"node ahead of the chain": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			f.Epochs[3] = model.Epoch{Index: 3, Status: model.EpochStatus_Open}
		}, checkNameSealed},
		"open epoch bounds": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			mutateEpoch(f, 2, func(e *model.Epoch) { e.InputIndexUpperBound = 5 })
		}, checkNameSealed},
		"extra tournament in the node": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			f.Tournaments[common.HexToAddress("0xff")] = model.Tournament{}
		}, checkNameTournaments},
		"wrong tournament parent": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			f.Tournaments[sampleInner] = model.Tournament{Address: sampleInner}
		}, checkNameTournaments},
		"accepted on chain, not in the node": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			mutateEpoch(f, 0, func(e *model.Epoch) { e.Status = model.EpochStatus_ClaimStaged })
		}, checkNameAccepted},
		"accepted in the node, not on chain": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			mutateEpoch(f, 1, func(e *model.Epoch) { e.Status = model.EpochStatus_ClaimAccepted })
		}, checkNameAccepted},
		"wrong claim transaction": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			mutateEpoch(f, 0, func(e *model.Epoch) { e.ClaimTransactionHash = hashPtr("0x21") })
		}, checkNameAccepted},
		"missing input":     {func(f *verifyFacts, _ map[uint64]common.Hash) { f.ChainInputCount = 3 }, checkNameInputs},
		"different payload": {func(f *verifyFacts, _ map[uint64]common.Hash) { f.Inputs[1].RawData = []byte("two") }, checkNameInputs},
		"input in the wrong epoch": {func(f *verifyFacts, _ map[uint64]common.Hash) { f.Inputs[0].EpochIndex = 1 },
			checkNameInputs},
		"input not on chain": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			f.Inputs = append(f.Inputs, model.Input{Index: 2, Status: model.InputCompletionStatus_Accepted})
		}, checkNameInputs},
		"Sling join disagrees": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			f.SlingJoins[1] = commitmentJoin{Commitment: common.HexToHash("0xbad"), FinalState: common.HexToHash("0xf0")}
		}, checkNameSlingCommitment},
		"Sling sentry claim disagrees": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			f.SlingSentry[1] = common.HexToHash("0xbad")
		}, checkNameSlingFinalState},
		"stage log and Sling join disagree": {func(_ *verifyFacts, c map[uint64]common.Hash) {
			c[0] = common.HexToHash("0xc1")
		}, checkNameSlingCommitment},
		"terminal status": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			f.App.Status = model.ApplicationStatus_InvalidOutputsRoot
		}, "application.status"},
		"wrong consensus on chain": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			f.ChainConsensus = common.HexToAddress("0x09")
		}, "application.identity"},
		"wrong template": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			f.App.TemplateHash = common.HexToHash("0x09")
		}, "application.identity"},
		"commitment not indexed": {func(f *verifyFacts, _ map[uint64]common.Hash) { f.Commitments = nil }, checkNameJoins},
		"commitment with another submitter": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			f.Commitments[0].SubmitterAddress = common.HexToAddress("0x07")
		}, checkNameJoins},
		"match not deleted in the node": {func(f *verifyFacts, _ map[uint64]common.Hash) {
			f.Matches[0].DeletionReason = model.MatchDeletionReason_NOT_DELETED
		}, checkNameMatches},
		"match not on chain": {func(f *verifyFacts, _ map[uint64]common.Hash) { f.ChainMatches = nil }, checkNameMatches},
	} {
		t.Run(name, func(t *testing.T) {
			f := sampleFacts()
			claims := sampleClaims()
			tc.mutate(f, claims)
			require.Equal(t, checkFail, statusOf(evaluate(f, sampleTarget(claims, false)), tc.check))
		})
	}
}

func TestStateChangeRule(t *testing.T) {
	f := sampleFacts()
	// Epoch 0 ends where it started: nothing compared changed state.
	mutateEpoch(f, 0, func(e *model.Epoch) { e.MachineHash = hashPtr("0x11") })
	f.Staged[0] = stagedEpoch{FinalState: common.HexToHash("0x11"), OutputsRoot: common.HexToHash("0xe0"), Block: 15}
	f.SlingJoins[0] = commitmentJoin{Commitment: common.HexToHash("0xc0"), FinalState: common.HexToHash("0x11")}
	f.SlingSentry[0] = common.HexToHash("0x11")
	require.Equal(t, checkNotTested, statusOf(evaluate(f, sampleTarget(sampleClaims(), false)), checkNameAgreement))
	require.Equal(t, checkFail, statusOf(evaluate(f, sampleTarget(sampleClaims(), true)), checkNameAgreement))
	report := &verifyReport{Checks: evaluate(f, sampleTarget(sampleClaims(), false))}
	summarize(report)
	require.True(t, report.Passed, "not-tested is reported, not failed")
	require.False(t, coverageOf(report, coverageAgreement).Tested)
}

// A mismatch in the Sling evidence means computation agreement was not
// tested, even when some other epoch changed state and matched.
func TestAgreementNeedsConsistentSlingEvidence(t *testing.T) {
	f := sampleFacts()
	claims := sampleClaims()
	claims[1] = common.HexToHash("0xdead")
	report := &verifyReport{Checks: evaluate(f, sampleTarget(claims, false))}
	summarize(report)
	require.False(t, report.Passed)
	require.False(t, coverageOf(report, coverageAgreement).Tested)
}

func TestNoSlingClaimsIsNotTested(t *testing.T) {
	f := sampleFacts()
	f.SlingJoins = map[uint64]commitmentJoin{}
	require.Equal(t, checkNotTested, statusOf(evaluate(f, sampleTarget(nil, false)), checkNameSlingCommitment))
}

func TestCompletionGaps(t *testing.T) {
	f := sampleFacts()
	f.App.LastTournamentCheckBlock = 90
	f.Inputs[1].Status = model.InputCompletionStatus_None
	delete(f.Tournaments, sampleInner)
	f.Matches = nil
	gaps, terminal := completionGaps(f, model.ApplicationStatus_OK)
	require.False(t, terminal)
	require.Len(t, gaps, 4, "%v", gaps)
	require.Contains(t, progressText(f), "inputs 1/2 processed")

	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_Diverged, model.ApplicationStatus_Failed} {
		f = sampleFacts()
		f.App.Status = status
		_, terminal = completionGaps(f, model.ApplicationStatus_OK)
		require.True(t, terminal, "%s ends the wait", status)
	}
	f = sampleFacts()
	_, terminal = completionGaps(f, model.ApplicationStatus_InvalidOutputsRoot)
	require.False(t, terminal, "an OK application may still reach the expected status")
}

func TestFingerprintFollowsEpochChanges(t *testing.T) {
	f := sampleFacts()
	before := fingerprint(f)
	require.Equal(t, before, fingerprint(sampleFacts()))
	mutateEpoch(f, 1, func(e *model.Epoch) { e.Status = model.EpochStatus_ClaimStaged })
	require.NotEqual(t, before, fingerprint(f))
}

// A root winner can be the node's own commitment, so it is not evidence of
// computation agreement.
func TestRootWinnerIsNotAgreementEvidence(t *testing.T) {
	f := sampleFacts()
	f.SlingJoins = map[uint64]commitmentJoin{}
	f.SlingSentry = map[uint64]common.Hash{}
	checks := evaluate(f, sampleTarget(nil, false))
	require.Equal(t, checkPass, statusOf(checks, "commitments.root_winner"))
	require.Equal(t, checkNotTested, statusOf(checks, checkNameAgreement))
}

// Sling evidence from the chain alone (a join and a sentry claim) is enough to
// test computation agreement.
func TestSlingJoinIsAgreementEvidence(t *testing.T) {
	checks := evaluate(sampleFacts(), sampleTarget(nil, true))
	require.Equal(t, checkPass, statusOf(checks, checkNameSlingCommitment))
	require.Equal(t, checkPass, statusOf(checks, checkNameSlingFinalState))
	require.Equal(t, checkPass, statusOf(checks, checkNameAgreement))
}

func TestVerifyRefusesTheNodeOwnSignerAsEvidence(t *testing.T) {
	signer := common.HexToAddress("0x06")
	_, err := verifyUntil(context.Background(), &verifyTarget{slingSigner: signer, rollupsSigner: signer}, time.Second)
	require.ErrorContains(t, err, "not independent evidence")
}

// A participant that exits ends a wait at once, even when the condition only
// reports retryable errors.
func TestWaitForStopsWhenAParticipantExits(t *testing.T) {
	dir := t.TempDir()
	sling, err := startChild("sling node", filepath.Join(dir, "log"), dir, os.Environ(), "sh", "-c", "exit 101")
	require.NoError(t, err)
	<-sling.done
	s := &session{sling: sling}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	start := time.Now()
	err = s.waitFor(ctx, "never", func() (bool, error) { return false, errors.New("retryable") })
	require.ErrorContains(t, err, "sling node exited early (code 101")
	require.Less(t, time.Since(start), 10*time.Second)
}

func TestWaitForStopsOnPermanentErrorsAndMinerFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s := &session{}
	start := time.Now()
	err := s.waitFor(ctx, "a transaction", func() (bool, error) { return false, permanent(errors.New("wrong transaction")) })
	require.ErrorContains(t, err, "wrong transaction")

	s.setMinerError(errors.New("mining failed 5 times"))
	err = s.waitFor(ctx, "never", func() (bool, error) { return false, nil })
	require.ErrorContains(t, err, "mining failed")
	require.Less(t, time.Since(start), 10*time.Second)
}

func TestExpandScenarios(t *testing.T) {
	list, err := expandScenarios("smoke")
	require.NoError(t, err)
	require.Equal(t, smokeScenarios, list)
	list, err = expandScenarios(" echo/simple , honeypot/stf_all ")
	require.NoError(t, err)
	require.Equal(t, []string{"echo/simple", "honeypot/stf_all"}, list)
	for _, bad := range []string{"", "echo", "yield/stf_all", "/simple"} {
		_, err = expandScenarios(bad)
		require.Error(t, err, bad)
	}
	all, err := expandScenarios("all")
	require.NoError(t, err)
	for _, item := range all {
		require.False(t, strings.HasPrefix(item, "yield/"), item)
	}
	require.Contains(t, all, "echo/stf_all")
	require.Contains(t, all, "honeypot/stf_revert", "replaces the out-of-scope yield/stf_revert")
}

func TestPrepareCaseDir(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()

	missing := filepath.Join(root, "missing")
	m, aside, err := prepareCaseDir(missing, now)
	require.NoError(t, err)
	require.Nil(t, m)
	require.Empty(t, aside)

	empty := filepath.Join(root, "empty")
	require.NoError(t, os.Mkdir(empty, 0o755))
	_, aside, err = prepareCaseDir(empty, now)
	require.NoError(t, err)
	require.Empty(t, aside)
	require.NoDirExists(t, empty, "an empty directory is removed so capture can create it")

	golden := filepath.Join(root, "golden")
	require.NoError(t, os.Mkdir(golden, 0o755))
	require.NoError(t, writeManifest(golden, validManifest()))
	m, aside, err = prepareCaseDir(golden, now)
	require.NoError(t, err)
	require.NotNil(t, m, "a golden case is reused")
	require.Empty(t, aside)

	// A failed capture (no manifest) and a non-golden case are set aside, not deleted.
	failed := filepath.Join(root, "failed")
	require.NoError(t, os.Mkdir(failed, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(failed, "harness.log"), []byte("boom"), 0o600))
	_, aside, err = prepareCaseDir(failed, now)
	require.NoError(t, err)
	require.Equal(t, failed+".failed-20260929T120000Z", aside)
	require.FileExists(t, filepath.Join(aside, "harness.log"))
	require.NoDirExists(t, failed)

	notGolden := filepath.Join(root, "not-golden")
	require.NoError(t, os.Mkdir(notGolden, 0o755))
	m = validManifest()
	m.Golden = false
	m.Provenance.HarnessExitCode = 1
	require.NoError(t, writeManifest(notGolden, m))
	m, aside, err = prepareCaseDir(notGolden, now)
	require.NoError(t, err)
	require.Nil(t, m)
	require.NotEmpty(t, aside)
}

func TestCleanKeepsCasesAndRemovesFailedCaptures(t *testing.T) {
	root := t.TempDir()
	cases := filepath.Join(root, "cases")
	for _, dir := range []string{"echo-simple/runs/r1", "echo-simple.failed-20260929T120000Z", "honeypot-stf_all"} {
		require.NoError(t, os.MkdirAll(filepath.Join(cases, dir), 0o755))
	}
	live := filepath.Join(root, "live", "l1")
	require.NoError(t, os.MkdirAll(live, 0o755))

	result, err := runClean(context.Background(), &cleanOptions{casesDir: cases, liveDir: filepath.Join(root, "live")})
	require.NoError(t, err)
	require.Len(t, result.Paths, 3)
	require.DirExists(t, filepath.Join(cases, "echo-simple"), "captured cases stay")
	require.DirExists(t, filepath.Join(cases, "honeypot-stf_all"))
	require.NoDirExists(t, filepath.Join(cases, "echo-simple", "runs"))
	require.NoDirExists(t, filepath.Join(cases, "echo-simple.failed-20260929T120000Z"))
	require.NoDirExists(t, filepath.Join(root, "live"))

	result, err = runClean(context.Background(), &cleanOptions{casesDir: cases, liveDir: filepath.Join(root, "live"), cases: true})
	require.NoError(t, err)
	require.Len(t, result.Paths, 2)
	require.NoDirExists(t, filepath.Join(cases, "honeypot-stf_all"))
}

func TestEstimateTexts(t *testing.T) {
	require.Equal(t, "about 40 s", aboutText(40*time.Second))
	require.Equal(t, "about 9 min", aboutText(9*time.Minute+20*time.Second))
	require.Equal(t, "1m30s of about 2 min", elapsedText(90*time.Second, 2*time.Minute))
	require.Equal(t, "3m0s so far, longer than the 2 min expected", elapsedText(3*time.Minute, 2*time.Minute))
}

func TestLastLogLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "harness.log")
	require.Equal(t, "", lastLogLine(path), "a missing log has no line")
	long := strings.Repeat("x", 200)
	require.NoError(t, os.WriteFile(path, []byte("first\n\x1b[32mINFO\x1b[0m sealed epoch 3\r\n\n"+long+"\n\n"), 0o600))
	line := lastLogLine(path)
	require.Len(t, []rune(line), maxLogLineRunes)
	require.True(t, strings.HasSuffix(line, "…"))
	require.NoError(t, os.WriteFile(path, []byte("first\n\x1b[32mINFO\x1b[0m sealed epoch 3\r\n\n"), 0o600))
	require.Equal(t, "INFO sealed epoch 3", lastLogLine(path))
}

func TestSuiteEstimates(t *testing.T) {
	cases := t.TempDir()
	// honeypot/stf_all has a golden case: reused. echo/simple failed before
	// and recorded how long its harness ran.
	golden := filepath.Join(cases, "honeypot-stf_all")
	require.NoError(t, os.Mkdir(golden, 0o755))
	require.NoError(t, writeManifest(golden, validManifest()))
	failed := filepath.Join(cases, "echo-simple"+failedCaseMarker+"20260929T120000Z")
	require.NoError(t, os.Mkdir(failed, 0o755))
	m := validManifest()
	m.Golden, m.Provenance.HarnessExitCode, m.Provenance.HarnessSeconds = false, 1, 150
	require.NoError(t, writeManifest(failed, m))

	estimate, source := harnessEstimate("echo/simple", cases)
	require.Equal(t, 150*time.Second, estimate)
	require.Equal(t, "as the last capture", source)
	estimate, _ = harnessEstimate("echo/unknown", cases)
	require.Equal(t, unknownHarnessDuration, estimate)

	plans := planSuite(&suiteOptions{casesDir: cases}, []string{"echo/simple", "honeypot/stf_all"},
		[]string{contOutputExecution})
	require.Equal(t, 150*time.Second+replayDuration, plans[0].total, "output-execution runs on honeypot only")
	require.Equal(t, 150*time.Second, plans[0].harness)
	require.Zero(t, plans[1].harness, "a golden case is reused")
	require.Equal(t, replayDuration+continuationDurations[contOutputExecution], plans[1].total)
	require.Contains(t, plans[1].text, "reuse the case")
}

func TestCheckJoinOrder(t *testing.T) {
	rollups := liveJoin{Party: partyRollups}
	sling := liveJoin{Party: partySling}
	require.Equal(t, checkPass, checkJoinOrder(orderRollupsFirst, []liveJoin{rollups}).Status)
	require.Equal(t, checkFail, checkJoinOrder(orderRollupsFirst, []liveJoin{sling}).Status)
	require.Equal(t, checkPass, checkJoinOrder(orderSlingFirst, []liveJoin{sling}).Status)
	require.Equal(t, checkNotTested, checkJoinOrder(orderConcurrent, []liveJoin{rollups}).Status)
	require.Equal(t, checkFail, checkJoinOrder(orderConcurrent, nil).Status, "nobody joined")
	require.Equal(t, checkFail, checkJoinOrder(orderRollupsFirst, []liveJoin{rollups, sling}).Status,
		"a second join means different commitments")
}

// pagedAPI serves cartesi_listInputs from pages; total is the reported count.
func pagedAPI(t *testing.T, total uint64, pages [][]model.Input) *nodeAPI {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID any `json:"id"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		page := []model.Input{}
		if calls < len(pages) {
			page = pages[calls]
		}
		calls++
		result, err := json.Marshal(map[string]any{"data": page, "pagination": map[string]uint64{"total_count": total}})
		require.NoError(t, err)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID,
			"result": json.RawMessage(result)}))
	}))
	t.Cleanup(server.Close)
	return newNodeAPI(server.URL)
}

func TestListAllReadsEveryPage(t *testing.T) {
	ctx := context.Background()
	inputs, err := pagedAPI(t, 3, [][]model.Input{{{Index: 0}, {Index: 1}}, {{Index: 2}}}).inputs(ctx, "app")
	require.NoError(t, err)
	require.Len(t, inputs, 3)

	_, err = pagedAPI(t, 3, [][]model.Input{{{Index: 0}}, {}}).inputs(ctx, "app")
	require.ErrorContains(t, err, "empty page")
}

func TestParseCLITransaction(t *testing.T) {
	hash, err := parseCLITransaction(`{"transaction_hash":"0x` + strings.Repeat("ab", 32) + `","status":"success"}` + "\n")
	require.NoError(t, err)
	require.Equal(t, common.HexToHash("0x"+strings.Repeat("ab", 32)), hash)
	_, err = parseCLITransaction(`{"status":"success"}`)
	require.ErrorContains(t, err, "no transaction hash")
	_, err = parseCLITransaction("not json")
	require.Error(t, err)
}

func TestFindTransfer(t *testing.T) {
	token, recipient, other := common.HexToAddress("0x10"), common.HexToAddress("0x20"), common.HexToAddress("0x30")
	transfer := func(address, to common.Address, amount int64) *types.Log {
		return &types.Log{Address: address, Topics: []common.Hash{transferEventTopic, common.BytesToHash(other.Bytes()),
			common.BytesToHash(to.Bytes())}, Data: common.LeftPadBytes(big.NewInt(amount).Bytes(), 32)}
	}
	receipt := &types.Receipt{Logs: []*types.Log{transfer(other, recipient, 1), transfer(token, other, 2),
		transfer(token, recipient, 1000)}}
	amount, ok := findTransfer(receipt, token, recipient)
	require.True(t, ok)
	require.Equal(t, int64(1000), amount.Int64())
	_, ok = findTransfer(&types.Receipt{}, token, recipient)
	require.False(t, ok)
}

func TestSlingArgsKeepTheKeyOffTheCommandLine(t *testing.T) {
	args := slingArgs(common.HexToAddress("0x01"), "/template", "http://rpc", anvilChainID, "/state", 1, "/state/key")
	require.Contains(t, args, "--web3-private-key-file")
	require.Equal(t, "/state/key", args[len(args)-1])
	require.Contains(t, args, "31337")
}

// The fake commitment must be structurally valid: its final state and proof
// rebuild its root, as the tournament contract checks on join.
func TestFakeCommitmentProofRebuildsTheRoot(t *testing.T) {
	const height = 48
	finalState, proof, left, right := fakeCommitmentNodes(height)
	require.Len(t, proof, height)
	node := finalState
	for _, sibling := range proof {
		// The final state is the last leaf: every sibling is on the left.
		node = crypto.Keccak256Hash(sibling[:], node.Bytes())
	}
	require.Equal(t, crypto.Keccak256Hash(left.Bytes(), right.Bytes()), node)
}

func TestOutputKind(t *testing.T) {
	parsed, err := outputs.OutputsMetaData.GetAbi()
	require.NoError(t, err)
	voucher, err := parsed.Pack("Voucher", common.HexToAddress("0x01"), big.NewInt(0), []byte("x"))
	require.NoError(t, err)
	notice, err := parsed.Pack("Notice", []byte("x"))
	require.NoError(t, err)
	require.Equal(t, outputVoucher, outputKind(voucher))
	require.Equal(t, outputNotice, outputKind(notice))
	require.Equal(t, textUnknown, outputKind([]byte{1, 2}))
}

func TestFullEpochPlans(t *testing.T) {
	plans := fullEpochPlans()
	require.Len(t, plans, 5)
	count := func(plan epochPlan) (accepted, rejected int) {
		for _, payload := range plan.payloads {
			if plannedAccept(payload) {
				accepted++
			} else {
				rejected++
			}
		}
		return accepted, rejected
	}
	for i, want := range [][2]int{{3, 0}, {2, 2}, {0, 3}, {0, 0}, {3, 0}} {
		accepted, rejected := count(plans[i])
		require.Equal(t, want, [2]int{accepted, rejected}, plans[i].name)
		require.Equal(t, i == 4, plans[i].fake, plans[i].name)
	}
}

func TestDecodeVoucher(t *testing.T) {
	parsed, err := outputs.OutputsMetaData.GetAbi()
	require.NoError(t, err)
	to := common.HexToAddress("0x90F79bf6EB2c4f870365E785982E1f101E93b906")
	voucher, err := parsed.Pack("Voucher", to, big.NewInt(1_000_000_000), []byte{})
	require.NoError(t, err)
	destination, value, err := decodeVoucher(voucher)
	require.NoError(t, err)
	require.Equal(t, to, destination)
	require.Equal(t, int64(1_000_000_000), value.Int64())
	notice, err := parsed.Pack("Notice", []byte("x"))
	require.NoError(t, err)
	_, _, err = decodeVoucher(notice)
	require.Error(t, err)
}

// The expected balance changes: the application passes the funding on to
// the voucher destination; the funder pays funding and gas; the executor
// pays gas.
func TestLedgerExpectedChanges(t *testing.T) {
	app, sender := common.HexToAddress("0xa0"), common.HexToAddress("0x03")
	funder, executor := common.HexToAddress("0x04"), common.HexToAddress("0x00")
	l := &voucherLedger{funder: funder, executor: executor, funded: big.NewInt(3_000), fundingGas: big.NewInt(21),
		transferred: map[common.Address]*big.Int{}, executorGas: new(big.Int)}
	for range 2 { // two of three vouchers executed; one failed after paying gas
		l.addExecution(&types.Receipt{Status: types.ReceiptStatusSuccessful, GasUsed: 10, EffectiveGasPrice: big.NewInt(2)},
			mustVoucher(t, sender, 1_000))
	}
	l.addExecution(&types.Receipt{Status: types.ReceiptStatusFailed, GasUsed: 5, EffectiveGasPrice: big.NewInt(2)},
		mustVoucher(t, sender, 1_000))
	want := l.expectedChanges(app)
	require.Equal(t, int64(1_000), want[app].Int64(), "one unexecuted voucher's value stays")
	require.Equal(t, int64(2_000), want[sender].Int64())
	require.Equal(t, int64(-3_021), want[funder].Int64())
	require.Equal(t, int64(-50), want[executor].Int64(), "the failed execution's gas counts too")
	require.Equal(t, 2, l.executions)
}

func mustVoucher(t *testing.T, to common.Address, value int64) []byte {
	t.Helper()
	parsed, err := outputs.OutputsMetaData.GetAbi()
	require.NoError(t, err)
	raw, err := parsed.Pack("Voucher", to, big.NewInt(value), []byte{})
	require.NoError(t, err)
	return raw
}

func TestBondPayout(t *testing.T) {
	bond := big.NewInt(1_000)
	payment, burned := bondPayout(bond, 1, big.NewInt(0))
	require.Equal(t, []int64{1_000, 0}, []int64{payment.Int64(), burned.Int64()}, "one join: the bond back")
	// Two joins, 200 refunded for dispute gas: 1,800 left; the winner gets one
	// bond and a tenth of the other 800; 720 burns.
	payment, burned = bondPayout(bond, 2, big.NewInt(200))
	require.Equal(t, []int64{1_080, 720}, []int64{payment.Int64(), burned.Int64()})
	// At most one bond left: all of it.
	payment, burned = bondPayout(bond, 2, big.NewInt(1_500))
	require.Equal(t, []int64{500, 0}, []int64{payment.Int64(), burned.Int64()})
}

func TestFormatWei(t *testing.T) {
	require.Equal(t, "8 gwei", formatWei(big.NewInt(8_000_000_000)))
	require.Equal(t, "0.3335 ETH", formatWei(big.NewInt(333_500_000_000_000_000)))
	require.Equal(t, "0.000021000000000123 ETH", formatWei(big.NewInt(21_000_000_000_123)))
	require.Equal(t, "-8 gwei", signedWei(big.NewInt(-8_000_000_000)))
	require.Equal(t, "+8 gwei", signedWei(big.NewInt(8_000_000_000)))
}

func TestIndicesText(t *testing.T) {
	require.Equal(t, "none", indicesText(nil))
	require.Equal(t, "0–6", indicesText([]uint64{0, 1, 2, 3, 4, 5, 6}))
	require.Equal(t, "1, 3, 4, 6", indicesText([]uint64{1, 3, 4, 6}))
	require.Equal(t, "0–2, 5", indicesText([]uint64{0, 1, 2, 5}))
	require.Equal(t, "epoch 5", epochsText([]uint64{5}))
	require.Equal(t, "no epoch", epochsText(nil))
}

// mixedJoinFacts has seven sealed epochs: the Sling node joined epochs 1, 3,
// 4, and 6 and the rollups node 0, 2, and 5; sentry claims for 0 to 5; the
// root of epoch 6 still running.
func mixedJoinFacts() (*verifyFacts, common.Address) {
	rollups, sling := common.HexToAddress("0x06"), common.HexToAddress("0x07")
	f := &verifyFacts{
		chainFacts: chainFacts{ChainJoins: map[common.Address][]commitmentJoin{}, Standings: map[uint64]rootStanding{},
			SlingJoins: map[uint64]commitmentJoin{}, SlingSentry: map[uint64]common.Hash{}},
		nodeFacts: nodeFacts{Epochs: map[uint64]model.Epoch{}},
	}
	for i := range uint64(7) {
		tournament := common.BigToAddress(new(big.Int).SetUint64(0xa0 + i))
		commitment, state := common.BigToHash(new(big.Int).SetUint64(0xc0+i)), common.BigToHash(new(big.Int).SetUint64(0xf0+i))
		initial := state // unchanged unless it had accepted inputs
		if i == 1 || i == 2 || i == 5 {
			initial = common.HexToHash("0x01")
		}
		f.Sealed = append(f.Sealed, sealedEpoch{Index: i, Tournament: tournament, InitialState: initial})
		joiner := rollups
		if i == 1 || i == 3 || i == 4 || i == 6 {
			joiner = sling
			f.SlingJoins[i] = commitmentJoin{Commitment: commitment, FinalState: state, Submitter: sling}
		}
		f.ChainJoins[tournament] = []commitmentJoin{{Commitment: commitment, FinalState: state, Submitter: joiner}}
		if i < 6 {
			f.SlingSentry[i] = state
			f.Standings[i] = rootStanding{Finished: true, HasWinner: true, Winner: commitment}
		} else {
			f.Standings[i] = rootStanding{}
		}
		f.Epochs[i] = model.Epoch{Index: i, Commitment: &commitment, MachineHash: &state}
	}
	return f, rollups
}

func TestSlingEvidenceDetailsAccountForEveryEpoch(t *testing.T) {
	f, rollups := mixedJoinFacts()
	commitments, matched := checkSlingCommitments(f, nil, rollups)
	finalStates, finalMatched := checkSlingFinalStates(f)
	for index := range finalMatched {
		matched[index] = true
	}
	winners := checkRootWinners(f)
	agreement := checkStateChange(f, matched, true)
	for _, check := range []checkResult{commitments, finalStates, winners, agreement} {
		t.Logf("%-24s %s: %s", check.Name, check.Status, check.Detail)
		require.Equal(t, checkPass, check.Status, check.Name)
	}
	require.Equal(t, "epochs 1, 3, 4, 6 match (Sling joins: 1, 3, 4, 6; Sling stage log: none); not compared: "+
		"epochs 0, 2, 5, which the rollups node joined first (the Sling node joined no other commitment)", commitments.Detail)
	require.Equal(t, "epochs 0–6 match (Sling sentry claims: 0–5; Sling joins: 1, 3, 4, 6)", finalStates.Detail)
	require.Equal(t, "finished roots: 0–5; the rollups commitment won 6 of 6; the root of epoch 6 has no winner yet",
		winners.Detail)
	require.Equal(t, "epochs that match the Sling node and changed state: 1, 2, 5; unchanged: 0, 3, 4, 6", agreement.Detail)
}

func TestDescribeBlocks(t *testing.T) {
	require.Equal(t, "", describeBlocks(nil))
	require.Equal(t, "24", describeBlocks([]uint64{24}))
	require.Equal(t, "24-26, 30", describeBlocks([]uint64{24, 25, 26, 30}))
	require.Equal(t, "1, 3, 5, 7, …", describeBlocks([]uint64{1, 3, 5, 7, 9, 11}))
}

func TestWrapWords(t *testing.T) {
	require.Equal(t, []string{"one two", "three", "0x0123456789abcdef0123", "four"},
		wrapWords("one two three 0x0123456789abcdef0123 four", 9))
	require.Empty(t, wrapWords("   ", 10))
}

func TestEpochExpectationCheck(t *testing.T) {
	inputs := map[uint64]model.Input{
		0: {Index: 0, EpochIndex: 1, Status: model.InputCompletionStatus_Accepted},
		1: {Index: 1, EpochIndex: 1, Status: model.InputCompletionStatus_Rejected},
	}
	inEpoch := []model.Input{inputs[0], inputs[1]}
	plan := epochExpectation{accepted: true, inputs: map[uint64]model.InputCompletionStatus{
		0: model.InputCompletionStatus_Accepted, 1: model.InputCompletionStatus_Rejected}}
	status, detail := plan.check(1, inputs, inEpoch, true, true)
	require.Equal(t, checkPass, status, detail)

	status, _ = plan.check(1, inputs, inEpoch, false, false)
	require.Equal(t, checkNotTested, status)

	status, detail = plan.check(1, inputs, inEpoch, true, false)
	require.Equal(t, checkFail, status)
	require.Equal(t, "not accepted on chain", detail)

	plan.inputs[1] = model.InputCompletionStatus_Accepted
	delete(plan.inputs, 0)
	status, detail = plan.check(1, inputs, inEpoch, true, true)
	require.Equal(t, checkFail, status)
	require.Equal(t, "input 1 is REJECTED in epoch 1, planned ACCEPTED; input 0 was not planned in this epoch", detail)
}

func TestInputCounts(t *testing.T) {
	require.Equal(t, "none", inputCounts(nil))
	require.Equal(t, "2 accepted, 1 rejected", inputCounts([]model.Input{
		{Status: model.InputCompletionStatus_Accepted}, {Status: model.InputCompletionStatus_Rejected},
		{Status: model.InputCompletionStatus_Accepted}}))
}
