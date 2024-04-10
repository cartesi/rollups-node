// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	PROJECT_NAME = "rollups-contracts"
	ZIP_FILE     = PROJECT_NAME + ".tar.gz"
)

// Deploy rollups-contracts to a local anvil instance
func deployRollupsContracts(ctx context.Context,
	rollupsContractsPath string) error {

	_, err := os.Stat(filepath.Join(rollupsContractsPath, "hardhat.config.ts"))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s does not contain a hardhat project: %v", rollupsContractsPath, err)
	}

	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := copyContracts(ctx, rollupsContractsPath, tmpDir); err != nil {
		return fmt.Errorf("failed to copy contracts: %v", err)
	}

	if err := deployContracts(ctx, tmpDir); err != nil {
		return fmt.Errorf("failed to deploy contracts: %v", err)
	}
	return nil
}

// Copy only the rollups hardhat project
func copyContracts(ctx context.Context, srcDir string, destDir string) error {
	cmd := exec.CommandContext(ctx, "cp", "-pr", srcDir, destDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command '%v' failed with %v", cmd.String(), err)
	}
	return nil
}

// Deploy rollups-contracts by using its own deployment script
func deployContracts(ctx context.Context, execDir string) error {
	cmdDir := execDir + "/rollups-contracts"

	cmd := exec.CommandContext(ctx, "pnpm", "install")
	cmd.Dir = cmdDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command '%v' failed with %v", cmd.String(), err)
	}

	cmd = exec.CommandContext(ctx, "pnpm", "deploy:development")
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "RPC_URL="+RPC_URL)
	cmd.Dir = cmdDir
	if VerboseLog {
		cmd.Stdout = os.Stdout
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command '%v' failed with %v", cmd.String(), err)
	}
	return nil
}
