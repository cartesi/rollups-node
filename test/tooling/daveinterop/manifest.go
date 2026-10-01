// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

const (
	manifestSchemaVersion = 1
	manifestFile          = "manifest.json"

	exitCodeObserved = "observed"
	exitCodeOperator = "operator"
	exitCodeNone     = "none"
)

// caseManifest describes one captured harness run. It holds facts and
// references only; secrets never belong here.
type caseManifest struct {
	Schema    int       `json:"schema"`
	Case      string    `json:"case"`
	CreatedAt time.Time `json:"created_at"`
	// Golden is true only when the harness exited with status zero and every
	// required artifact was captured and checked.
	Golden      bool            `json:"golden"`
	Provenance  caseProvenance  `json:"provenance"`
	Chain       caseChain       `json:"chain"`
	Application caseApplication `json:"application"`
	Deployments caseDeployments `json:"deployments"`
	Artifacts   caseArtifacts   `json:"artifacts"`
	Reference   caseReference   `json:"reference"`
	Notes       []string        `json:"notes,omitempty"`
}

type caseProvenance struct {
	Program           string `json:"program"`
	Scenario          string `json:"scenario"`
	RollupsRevision   string `json:"go_revision"`
	RollupsDirty      bool   `json:"go_dirty"`
	DaveRevision      string `json:"dave_revision"`
	DaveDirty         bool   `json:"dave_dirty"`
	SlingBinarySHA256 string `json:"dave_binary_sha256"`
	HarnessCommand    string `json:"harness_command"`
	HarnessExitCode   int    `json:"harness_exit_code"`
	// ExitCodeSource says where HarnessExitCode comes from: "observed" (this
	// tool ran the harness), "operator" (--harness-log), or "none" (an
	// imported dump without a log). Older manifests leave it empty.
	ExitCodeSource string `json:"exit_code_source,omitempty"`
	// HarnessSeconds is how long the harness ran; the suite uses it for its
	// time estimates.
	HarnessSeconds int    `json:"harness_seconds,omitempty"`
	AnvilVersion   string `json:"anvil_version"`
	TestInstance   string `json:"test_instance,omitempty"`
}

type caseChain struct {
	ChainID        uint64      `json:"chain_id"`
	Head           uint64      `json:"head"`
	HeadHash       common.Hash `json:"head_hash"`
	SlotsInAnEpoch int         `json:"slots_in_an_epoch"`
}

type caseApplication struct {
	Address      common.Address `json:"address"`
	Consensus    common.Address `json:"consensus"`
	DeployBlock  uint64         `json:"deploy_block"`
	TemplateHash common.Hash    `json:"template_hash"`
	// TemplateDir is relative to the case directory.
	TemplateDir string `json:"template_dir"`
}

type caseDeployments struct {
	InputBox          common.Address `json:"input_box"`
	DaveAppFactory    common.Address `json:"dave_app_factory"`
	Erc20Portal       common.Address `json:"erc20_portal"`
	TestFungibleToken common.Address `json:"test_fungible_token"`
}

type caseArtifacts struct {
	Dump       caseFile `json:"dump"`
	DaveLog    string   `json:"dave_log"`
	HarnessLog string   `json:"harness_log"`
}

// caseFile is a compressed artifact relative to the case directory.
type caseFile struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	RawSHA256 string `json:"raw_sha256"`
	RawSize   int64  `json:"raw_size"`
}

// caseReference holds the expected values built at capture time from the
// chain and the Sling node, never from the rollups node under test.
type caseReference struct {
	// Signer is the Sling node's signer; its joins and sentry claims on chain
	// are evidence of its own computation.
	Signer         common.Address `json:"signer"`
	SignerJoins    int            `json:"signer_joins"`
	SignerSentries int            `json:"signer_sentry_claims"`
	ClaimSource    string         `json:"claim_source"`
	Claims         []epochClaim   `json:"claims"`
	SealedEpochs   []sealedEpoch  `json:"sealed_epochs"`
	StagedEpochs   []stagedEpoch  `json:"staged_epochs"`
	InputCount     uint64         `json:"input_count"`
	Tournaments    int            `json:"tournaments"`
}

func readManifest(caseDir string) (*caseManifest, error) {
	data, err := os.ReadFile(filepath.Join(caseDir, manifestFile))
	if err != nil {
		return nil, err
	}
	var m caseManifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", manifestFile, err)
	}
	if err := m.validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", manifestFile, err)
	}
	return &m, nil
}

func writeManifest(caseDir string, m *caseManifest) error {
	if err := m.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(caseDir, manifestFile), append(data, '\n'), 0o644) //nolint:mnd,gosec
}

func (m *caseManifest) validate() error {
	var problems []string
	if m.Schema != manifestSchemaVersion {
		problems = append(problems, fmt.Sprintf("schema %d is not supported (want %d)", m.Schema, manifestSchemaVersion))
	}
	if m.Case == "" {
		problems = append(problems, "case is empty")
	}
	if m.Chain.ChainID == 0 || m.Chain.Head == 0 {
		problems = append(problems, "chain id and head are required")
	}
	if m.Application.Address == (common.Address{}) || m.Application.Consensus == (common.Address{}) {
		problems = append(problems, "application and consensus addresses are required")
	}
	if m.Deployments.InputBox == (common.Address{}) {
		problems = append(problems, "input box address is required")
	}
	for name, path := range map[string]string{
		"template_dir": m.Application.TemplateDir, "dump": m.Artifacts.Dump.Path,
	} {
		if err := checkRelativePath(path); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
		}
	}
	for name, path := range map[string]string{"dave_log": m.Artifacts.DaveLog, "harness_log": m.Artifacts.HarnessLog} {
		if path == "" {
			continue
		}
		if err := checkRelativePath(path); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
		}
	}
	if m.Artifacts.Dump.SHA256 == "" {
		problems = append(problems, "dump checksum is required")
	}
	seen := map[uint64]bool{}
	for _, claim := range m.Reference.Claims {
		if seen[claim.Epoch] {
			problems = append(problems, fmt.Sprintf("duplicate claim for epoch %d", claim.Epoch))
		}
		seen[claim.Epoch] = true
	}
	if m.Golden && m.Provenance.HarnessExitCode != 0 {
		problems = append(problems, "a golden case needs harness exit code 0")
	}
	if m.Golden && m.Provenance.ExitCodeSource != "" && m.Provenance.ExitCodeSource != exitCodeObserved {
		problems = append(problems, "a golden case needs an observed harness exit code")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func checkRelativePath(path string) error {
	if path == "" {
		return errors.New("path is empty")
	}
	if filepath.IsAbs(path) {
		return errors.New("path must be relative to the case directory")
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return errors.New("path must stay inside the case directory")
	}
	return nil
}

// claimMap indexes the Sling stage claims by epoch.
func (r caseReference) claimMap() map[uint64]common.Hash {
	return claimsByEpoch(r.Claims)
}

func claimsByEpoch(claims []epochClaim) map[uint64]common.Hash {
	byEpoch := make(map[uint64]common.Hash, len(claims))
	for _, claim := range claims {
		byEpoch[claim.Epoch] = claim.Commitment
	}
	return byEpoch
}
