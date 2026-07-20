// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-machine-tool/accountdrive"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
)

const defaultInputPageSize = uint64(500)

func main() {
	config.SetDefaults()
	if err := newRootCommand().ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "cartesi-rollups-machine-tool",
		Short: "Rollups-aware helper for replaying machines and generating withdrawal proofs",
	}
	root.AddCommand(newReplayCommand())
	root.AddCommand(newProveCommand())
	return root
}

func newReplayCommand() *cobra.Command {
	var opts replayOptions
	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Replay accepted inputs from the node database into a machine template",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.HasToEpoch = cmd.Flags().Changed("to-epoch")
			opts.HasToInputIndex = cmd.Flags().Changed("to-input-index")
			return runReplay(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Template, "template", "", "Stored machine template path")
	cmd.Flags().StringVar(&opts.Application, "application", "", "Application name or address")
	cmd.Flags().StringVar(&opts.DatabaseConnection, "database-connection", "", "Database connection string")
	cmd.Flags().StringVar(&opts.Store, "store", "", "Output stored machine path")
	cmd.Flags().StringVar(&opts.CartesiMachine, "cartesi-machine", "cartesi-machine", "cartesi-machine executable")
	cmd.Flags().StringVar(&opts.Lua, "lua", "lua5.4", "Lua executable used by Dave/PRT replay")
	cmd.Flags().StringVar(&opts.CartesiSDKRoot, "cartesi-sdk-root", "", "Cartesi SDK root used to resolve Lua modules")
	cmd.Flags().Uint64Var(&opts.ToEpoch, "to-epoch", 0, "Replay accepted inputs in epochs up to this epoch")
	cmd.Flags().Uint64Var(&opts.ToInputIndex, "to-input-index", 0, "Replay accepted inputs up to this input index")
	cobra.CheckErr(cmd.MarkFlagRequired("template"))
	cobra.CheckErr(cmd.MarkFlagRequired("application"))
	cobra.CheckErr(cmd.MarkFlagRequired("store"))
	return cmd
}

type replayOptions struct {
	Template           string
	Application        string
	DatabaseConnection string
	Store              string
	CartesiMachine     string
	Lua                string
	CartesiSDKRoot     string
	ToEpoch            uint64
	ToInputIndex       uint64
	HasToEpoch         bool
	HasToInputIndex    bool
}

func runReplay(ctx context.Context, opts replayOptions) error {
	if opts.DatabaseConnection == "" {
		dsn, err := config.GetDatabaseConnection()
		if err != nil {
			return fmt.Errorf("database connection is required for replay: %w", err)
		}
		opts.DatabaseConnection = dsn.Raw()
	}
	if opts.HasToEpoch == opts.HasToInputIndex {
		return errors.New("exactly one replay target is required: --to-epoch or --to-input-index")
	}

	repo, err := factory.NewRepositoryFromConnectionString(ctx, opts.DatabaseConnection)
	if err != nil {
		return fmt.Errorf("open repository: %w", err)
	}
	defer repo.Close()

	app, err := repo.GetApplication(ctx, opts.Application)
	if err != nil {
		return fmt.Errorf("get application: %w", err)
	}
	if app == nil {
		return fmt.Errorf("application %q not found", opts.Application)
	}

	inputs, lastInputIndex, err := collectReplayInputs(ctx, repo, opts)
	if err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "cartesi-rollups-machine-tool-replay-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmp) //nolint:errcheck

	if err := replayInputs(ctx, opts, tmp, app, inputs); err != nil {
		return err
	}

	root, err := readStoredMachineRoot(ctx, opts.CartesiMachine, opts.Store)
	if err != nil {
		return err
	}
	summary := struct {
		ProcessedInputs int    `json:"processed_inputs"`
		LastInputIndex  string `json:"last_input_index,omitempty"`
		MachineRoot     string `json:"machine_root"`
		Store           string `json:"store"`
	}{
		ProcessedInputs: len(inputs),
		MachineRoot:     root,
		Store:           opts.Store,
	}
	if lastInputIndex != nil {
		summary.LastInputIndex = fmt.Sprintf("0x%x", *lastInputIndex)
	}
	return json.NewEncoder(os.Stdout).Encode(summary)
}

func replayInputs(
	ctx context.Context,
	opts replayOptions,
	tmp string,
	app *model.Application,
	inputs []*model.Input,
) error {
	if app.IsDaveConsensus() && len(inputs) > 0 {
		return replayDaveInputs(ctx, opts, tmp, inputs)
	}
	return replayInputsBatch(ctx, opts, tmp, inputs)
}

func replayInputsBatch(ctx context.Context, opts replayOptions, tmp string, inputs []*model.Input) error {
	if _, err := writeReplayInputFiles(tmp, inputs); err != nil {
		return err
	}

	args := []string{
		"--quiet",
		"--no-revert",
		"--load=" + opts.Template,
		fmt.Sprintf("--cmio-advance-state=input:%s,input_index_begin:0,input_index_end:%d",
			filepath.Join(tmp, "input-%i.bin"), len(inputs)),
		"--store=" + opts.Store,
	}
	return runCommand(ctx, opts.CartesiMachine, args...)
}

func collectReplayInputs(
	ctx context.Context,
	repo repository.Repository,
	opts replayOptions,
) ([]*model.Input, *uint64, error) {
	status := model.InputCompletionStatus_Accepted
	filter := repository.InputFilter{Status: &status}
	var offset uint64
	var result []*model.Input
	var lastInputIndex *uint64
	for {
		inputs, _, err := repo.ListInputs(ctx, opts.Application, filter,
			repository.Pagination{Limit: defaultInputPageSize, Offset: offset}, false)
		if err != nil {
			return nil, nil, fmt.Errorf("list inputs: %w", err)
		}
		if len(inputs) == 0 {
			break
		}
		for _, input := range inputs {
			if opts.HasToEpoch && input.EpochIndex > opts.ToEpoch {
				return result, lastInputIndex, nil
			}
			if opts.HasToInputIndex && input.Index > opts.ToInputIndex {
				return result, lastInputIndex, nil
			}
			result = append(result, input)
			idx := input.Index
			lastInputIndex = &idx
		}
		if uint64(len(inputs)) < defaultInputPageSize {
			break
		}
		offset += uint64(len(inputs))
	}
	return result, lastInputIndex, nil
}

func newProveCommand() *cobra.Command {
	prove := &cobra.Command{
		Use:   "prove",
		Short: "Generate Rollups proof files from a stored machine",
	}
	prove.AddCommand(newProveAccountsDriveCommand())
	return prove
}

func newProveAccountsDriveCommand() *cobra.Command {
	var opts proveAccountsDriveOptions
	cmd := &cobra.Command{
		Use:   "accounts-drive",
		Short: "Generate accounts-drive root and account withdrawal proofs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProveAccountsDrive(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Snapshot, "snapshot", "", "Stored machine snapshot path")
	cmd.Flags().StringVar(&opts.Account, "account", "", "Account address to prove")
	cmd.Flags().Uint64Var(&opts.AccountsDriveStartIndex, "accounts-drive-start-index", 0, "Accounts-drive start index")
	cmd.Flags().Uint8Var(&opts.Log2MaxNumOfAccounts, "log2-max-num-of-accounts", accountdrive.DefaultLog2MaxAccount,
		"Log2 of max number of accounts")
	cmd.Flags().Uint8Var(&opts.Log2LeavesPerAccount, "log2-leaves-per-account", 0, "Log2 of leaves per account")
	cmd.Flags().StringVar(&opts.OutDriveRootProof, "out-drive-root-proof", "", "Output JSON for prove-drive-root")
	cmd.Flags().StringVar(&opts.OutWithdrawProof, "out-withdraw-proof", "", "Output JSON for withdraw")
	cmd.Flags().StringVar(&opts.CartesiMachine, "cartesi-machine", "cartesi-machine", "cartesi-machine executable")
	cobra.CheckErr(cmd.MarkFlagRequired("snapshot"))
	cobra.CheckErr(cmd.MarkFlagRequired("account"))
	cobra.CheckErr(cmd.MarkFlagRequired("out-drive-root-proof"))
	cobra.CheckErr(cmd.MarkFlagRequired("out-withdraw-proof"))
	return cmd
}

type proveAccountsDriveOptions struct {
	Snapshot                string
	Account                 string
	AccountsDriveStartIndex uint64
	Log2MaxNumOfAccounts    uint8
	Log2LeavesPerAccount    uint8
	OutDriveRootProof       string
	OutWithdrawProof        string
	CartesiMachine          string
}

func runProveAccountsDrive(ctx context.Context, opts proveAccountsDriveOptions) error {
	if !common.IsHexAddress(opts.Account) {
		return fmt.Errorf("invalid account address %q", opts.Account)
	}
	account := common.HexToAddress(opts.Account)
	log2DriveSize := accountdrive.Log2AccountSize + opts.Log2MaxNumOfAccounts + opts.Log2LeavesPerAccount
	driveSize, err := accountdrive.DriveSize(opts.Log2MaxNumOfAccounts, opts.Log2LeavesPerAccount)
	if err != nil {
		return err
	}
	driveStart := opts.AccountsDriveStartIndex << log2DriveSize

	drivePath, err := findStoredDrive(opts.Snapshot, driveStart, driveSize)
	if err != nil {
		return err
	}
	drive, err := os.ReadFile(drivePath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("read accounts drive %s: %w", drivePath, err)
	}
	if uint64(len(drive)) > driveSize {
		drive = drive[:driveSize]
	}

	accountProof, err := accountdrive.BuildProof(drive, account, opts.Log2MaxNumOfAccounts, opts.Log2LeavesPerAccount)
	if err != nil {
		return err
	}
	machineProof, err := generateMachineProof(ctx, opts.CartesiMachine, opts.Snapshot, driveStart, log2DriveSize)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strip0x(machineProof.TargetHash), accountProof.DriveRoot.Hex()[2:]) {
		return fmt.Errorf("accounts-drive root mismatch: machine proof has %s, local drive has %s",
			ensure0x(machineProof.TargetHash), accountProof.DriveRoot.Hex())
	}

	if err := writeDriveRootProof(opts.OutDriveRootProof, machineProof); err != nil {
		return err
	}
	if err := writeWithdrawProof(opts.OutWithdrawProof, accountProof); err != nil {
		return err
	}

	summary := struct {
		Account                 common.Address `json:"account"`
		AccountIndex            string         `json:"account_index"`
		AccountsDriveMerkleRoot string         `json:"accounts_drive_merkle_root"`
		MachineRoot             string         `json:"machine_root"`
		DriveRootProofFile      string         `json:"drive_root_proof_file"`
		WithdrawProofFile       string         `json:"withdraw_proof_file"`
	}{
		Account:                 account,
		AccountIndex:            fmt.Sprintf("0x%x", accountProof.AccountIndex),
		AccountsDriveMerkleRoot: accountProof.DriveRoot.Hex(),
		MachineRoot:             ensure0x(machineProof.RootHash),
		DriveRootProofFile:      opts.OutDriveRootProof,
		WithdrawProofFile:       opts.OutWithdrawProof,
	}
	return json.NewEncoder(os.Stdout).Encode(summary)
}

type storedMachineConfig struct {
	Config struct {
		FlashDrive []struct {
			BackingStore struct {
				DataFilename string `json:"data_filename"`
			} `json:"backing_store"`
			Length uint64 `json:"length"`
			Start  uint64 `json:"start"`
		} `json:"flash_drive"`
	} `json:"config"`
}

func findStoredDrive(snapshot string, start uint64, length uint64) (string, error) {
	raw, err := os.ReadFile(filepath.Join(snapshot, "config.json")) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("read stored machine config: %w", err)
	}
	var cfg storedMachineConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("parse stored machine config: %w", err)
	}
	for _, drive := range cfg.Config.FlashDrive {
		if drive.Start == start && drive.Length >= length {
			return filepath.Join(snapshot, strings.TrimPrefix(drive.BackingStore.DataFilename, "./")), nil
		}
	}
	return "", fmt.Errorf("accounts drive not found in stored machine: start=0x%x length=0x%x", start, length)
}

type cartesiMachineProof struct {
	TargetAddress  uint64   `json:"target_address"`
	Log2TargetSize uint8    `json:"log2_target_size"`
	Log2RootSize   uint8    `json:"log2_root_size"`
	TargetHash     string   `json:"target_hash"`
	SiblingHashes  []string `json:"sibling_hashes"`
	RootHash       string   `json:"root_hash"`
}

func generateMachineProof(
	ctx context.Context,
	cartesiMachine string,
	snapshot string,
	address uint64,
	log2Size uint8,
) (*cartesiMachineProof, error) {
	tmp, err := os.CreateTemp("", "cartesi-rollups-machine-proof-*.json")
	if err != nil {
		return nil, fmt.Errorf("create proof temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath) //nolint:errcheck

	args := []string{
		"--quiet",
		"--no-revert",
		"--load=" + snapshot,
		fmt.Sprintf("--initial-proof=address:0x%x,log2_size:%d,filename:%s", address, log2Size, tmpPath),
		"--",
		"/bin/true",
	}
	if err := runCommand(ctx, cartesiMachine, args...); err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(tmpPath) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read machine proof: %w", err)
	}
	var proof cartesiMachineProof
	if err := json.Unmarshal(raw, &proof); err != nil {
		return nil, fmt.Errorf("parse machine proof: %w", err)
	}

	// convert the file hashes from base64 to hex
	proof.RootHash, err = base64ToHex(proof.RootHash)
	if err != nil {
		return nil, err
	}
	proof.TargetHash, err = base64ToHex(proof.TargetHash)
	if err != nil {
		return nil, err
	}
	for i := range proof.SiblingHashes {
		proof.SiblingHashes[i], err = base64ToHex(proof.SiblingHashes[i])
		if err != nil {
			return nil, err
		}
	}

	return &proof, nil
}

func writeDriveRootProof(path string, proof *cartesiMachineProof) error {
	out := struct {
		AccountsDriveMerkleRoot string   `json:"accounts_drive_merkle_root"`
		Proof                   []string `json:"proof"`
	}{
		AccountsDriveMerkleRoot: ensure0x(proof.TargetHash),
		Proof:                   make([]string, len(proof.SiblingHashes)),
	}
	for i, sibling := range proof.SiblingHashes {
		out.Proof[i] = ensure0x(sibling)
	}
	return writeJSON(path, out)
}

func writeWithdrawProof(path string, proof *accountdrive.Proof) error {
	out := struct {
		Account             string   `json:"account"`
		AccountIndex        string   `json:"account_index"`
		AccountRootSiblings []string `json:"account_root_siblings"`
	}{
		Account:             hexutil.Encode(proof.Account[:]),
		AccountIndex:        fmt.Sprintf("0x%x", proof.AccountIndex),
		AccountRootSiblings: make([]string, len(proof.Siblings)),
	}
	for i, sibling := range proof.Siblings {
		out.AccountRootSiblings[i] = sibling.Hex()
	}
	return writeJSON(path, out)
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func runCommand(ctx context.Context, name string, args ...string) error {
	return runCommandWithEnv(ctx, name, nil, args...)
}

func runCommandWithEnv(ctx context.Context, name string, env []string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = ioDiscardUnlessDebug()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s failed: %w\n%s", name, strings.Join(args, " "), err, stderr.String())
	}
	return nil
}

func ioDiscardUnlessDebug() *bytes.Buffer {
	return &bytes.Buffer{}
}

func readStoredMachineRoot(ctx context.Context, cartesiMachine string, store string) (string, error) {
	const minProofLog2Size = 5
	proof, err := generateMachineProof(ctx, cartesiMachine, store, 0, minProofLog2Size)
	if err != nil {
		return "", fmt.Errorf("read stored machine root: %w", err)
	}
	return ensure0x(proof.RootHash), nil
}

func ensure0x(s string) string {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return "0x" + s[2:]
	}
	return "0x" + s
}

func strip0x(s string) string {
	return strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
}

func base64ToHex(s string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("expected %q to be a hash in base64. %w", s, err)
	}
	if len(raw) != common.HashLength {
		return "", fmt.Errorf("expected %q to decode to %d bytes, got %d", s, common.HashLength, len(raw))
	}
	return common.BytesToHash(raw).Hex(), nil
}
