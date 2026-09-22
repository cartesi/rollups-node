// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-machine-tool/accountdrive"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/manager"
	"github.com/cartesi/rollups-node/internal/replay"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
)

const defaultInputPageSize = uint64(500)

func main() {
	config.SetDefaults()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := newRootCommand(logger).ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand(logger *slog.Logger) *cobra.Command {
	root := &cobra.Command{
		Use:   "cartesi-rollups-machine-tool",
		Short: "Rollups-aware helper for replaying machines and generating withdrawal proofs",
	}
	root.AddCommand(newReplayCommand(logger))
	root.AddCommand(newProveCommand(logger))
	return root
}

func newReplayCommand(logger *slog.Logger) *cobra.Command {
	var opts replayOptions
	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Replay completed inputs from the node database into a machine template",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.HasToEpoch = cmd.Flags().Changed("to-epoch")
			opts.HasToInputIndex = cmd.Flags().Changed("to-input-index")
			return runReplay(cmd.Context(), logger, opts)
		},
	}
	cmd.Flags().StringVar(&opts.Application, "application", "", "Application name or address")
	cmd.Flags().StringVar(&opts.DatabaseConnection, "database-connection", "", "Database connection string")
	cmd.Flags().StringVar(&opts.Store, "store", "", "Output stored machine path")
	cmd.Flags().Uint64Var(&opts.ToEpoch, "to-epoch", 0, "Replay completed inputs in epochs up to this epoch")
	cmd.Flags().Uint64Var(&opts.ToInputIndex, "to-input-index", 0, "Replay completed inputs up to this input index")
	cobra.CheckErr(cmd.MarkFlagRequired("application"))
	cobra.CheckErr(cmd.MarkFlagRequired("store"))
	return cmd
}

type replayOptions struct {
	Application        string
	DatabaseConnection string
	Store              string
	ToEpoch            uint64
	ToInputIndex       uint64
	HasToEpoch         bool
	HasToInputIndex    bool
}

func runReplay(ctx context.Context, logger *slog.Logger, opts replayOptions) error {
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

	toInputExclusive, err := resolveReplayUpperBound(ctx, repo, app.Name, opts)
	if err != nil {
		return err
	}

	lastInput, err := repo.GetLastProcessedInput(ctx, opts.Application)
	if err != nil {
		return fmt.Errorf("get last processed input: %w", err)
	}
	if lastInput != nil && lastInput.Status.IsTerminal() && toInputExclusive > lastInput.Index {
		return fmt.Errorf(
			"cannot snapshot: replay range reaches the application's terminal input 0x%x (status %s). Replay up to input 0x%x instead",
			lastInput.Index, lastInput.Status, lastInput.Index,
		)
	}

	instance, err := manager.NewMachineInstance(ctx, app, logger, true)
	if err != nil {
		return fmt.Errorf("create machine instance: %w", err)
	}
	defer instance.Close()

	result, err := replay.Run(ctx, repo, instance, replay.Options{
		Application:      app,
		FromInput:        0,
		ToInputExclusive: toInputExclusive,
		BatchSize:        defaultInputPageSize,
		Verification:     repository.ReplayVerificationCanonical,
	})
	if err != nil {
		return fmt.Errorf("replay: %w", err)
	}
	if err := instance.CreateSnapshot(ctx, instance.ProcessedInputs(), opts.Store); err != nil {
		return fmt.Errorf("store machine: %w", err)
	}

	root, err := instance.Hash(ctx)
	if err != nil {
		return fmt.Errorf("read machine root: %w", err)
	}
	summary := struct {
		ProcessedInputs uint64 `json:"processed_inputs"`
		LastInputIndex  string `json:"last_input_index,omitempty"`
		MachineRoot     string `json:"machine_root"`
		Store           string `json:"store"`
	}{
		ProcessedInputs: result.ReplayedInputs,
		MachineRoot:     common.BytesToHash(root[:]).Hex(),
		Store:           opts.Store,
	}
	if result.ReplayedInputs > 0 {
		summary.LastInputIndex = fmt.Sprintf("0x%x", toInputExclusive-1)
	}
	return json.NewEncoder(os.Stdout).Encode(summary)
}

// resolveReplayUpperBound resolves the exclusive upper input bound of the
// replay range: the end of the requested epoch, or one past the requested
// input index.
func resolveReplayUpperBound(
	ctx context.Context,
	repo repository.Repository,
	application string,
	opts replayOptions,
) (uint64, error) {
	if opts.HasToEpoch {
		epoch, err := repo.GetEpoch(ctx, application, opts.ToEpoch)
		if err != nil {
			return 0, fmt.Errorf("get epoch %d: %w", opts.ToEpoch, err)
		}
		if epoch == nil {
			return 0, fmt.Errorf("epoch %d not found for application %q", opts.ToEpoch, application)
		}
		return epoch.InputIndexUpperBound, nil
	}
	if opts.ToInputIndex == ^uint64(0) {
		return 0, fmt.Errorf("to-input-index overflow")
	}
	return opts.ToInputIndex + 1, nil
}

func newProveCommand(logger *slog.Logger) *cobra.Command {
	prove := &cobra.Command{
		Use:   "prove",
		Short: "Generate Rollups proof files from a stored machine",
	}
	prove.AddCommand(newProveAccountsDriveCommand(logger))
	return prove
}

func newProveAccountsDriveCommand(logger *slog.Logger) *cobra.Command {
	var opts proveAccountsDriveOptions
	cmd := &cobra.Command{
		Use:   "accounts-drive",
		Short: "Generate accounts-drive root and account withdrawal proofs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProveAccountsDrive(cmd.Context(), logger, opts)
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
}

func runProveAccountsDrive(ctx context.Context, logger *slog.Logger, opts proveAccountsDriveOptions) error {
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

	m, err := machine.Load(ctx, logger, machine.DefaultConfig(opts.Snapshot))
	if err != nil {
		return fmt.Errorf("load stored machine: %w", err)
	}
	defer m.Close()

	machineProof, err := m.GetProof(ctx, driveStart, int32(log2DriveSize), 64) //nolint:mnd // full machine memory
	if err != nil {
		return fmt.Errorf("get accounts-drive proof: %w", err)
	}
	if common.Hash(machineProof.TargetHash) != accountProof.DriveRoot {
		return fmt.Errorf("accounts-drive root mismatch: machine proof has %s, local drive has %s",
			common.Hash(machineProof.TargetHash).Hex(), accountProof.DriveRoot.Hex())
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
		MachineRoot:             common.BytesToHash(machineProof.RootHash[:]).Hex(),
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

func writeDriveRootProof(path string, proof *machine.MemoryProof) error {
	out := struct {
		AccountsDriveMerkleRoot string   `json:"accounts_drive_merkle_root"`
		Proof                   []string `json:"proof"`
	}{
		AccountsDriveMerkleRoot: common.BytesToHash(proof.TargetHash[:]).Hex(),
		Proof:                   make([]string, len(proof.Siblings)),
	}
	for i, sibling := range proof.Siblings {
		out.Proof[i] = common.BytesToHash(sibling[:]).Hex()
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
