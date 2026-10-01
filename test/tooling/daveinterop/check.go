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
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"
)

const checkTimeout = 10 * time.Second

type checkOptions struct {
	capability string
	daveRoot   string
	slingBin   string
	rpc        string
	dbAdmin    string
	nodeBin    string
	cliBin     string
}

type preflightItem struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	OK       bool   `json:"ok"`
	Detail   string `json:"detail"`
}

type preflightReport struct {
	Capability string          `json:"capability"`
	OK         bool            `json:"ok"`
	Items      []preflightItem `json:"items"`
}

func newCheckCommand() *cobra.Command {
	opts := &checkOptions{}
	cmd := &cobra.Command{
		Use:   "check [--for capture|replay|suite|run-sling]",
		Short: "Check the prerequisites of one capability; install nothing",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCheck(cmd.Context(), cmd, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.capability, "for", "replay", "capability to check: capture, replay, suite, or run-sling")
	flags.StringVar(&opts.daveRoot, "dave-root", os.Getenv("DAVE_ROOT"), "prepared Dave tree (default: $DAVE_ROOT)")
	flags.StringVar(&opts.slingBin, "sling-bin", defaultSlingBin(),
		"Sling node executable (default: $DAVE_NODE_BIN, or its build in the Dave tree)")
	flags.StringVar(&opts.rpc, "rpc", envOr("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://127.0.0.1:8545"), "chain for run-sling")
	flags.StringVar(&opts.dbAdmin, "db-admin", defaultDatabaseURL(), "Postgres URL for replay")
	flags.StringVar(&opts.nodeBin, "node-bin", "./cartesi-rollups-node", "rollups node binary")
	flags.StringVar(&opts.cliBin, "cli-bin", "./cartesi-rollups-cli", "rollups CLI binary")
	return cmd
}

func runCheck(ctx context.Context, cmd *cobra.Command, opts *checkOptions) error {
	report := &preflightReport{Capability: opts.capability, OK: true}
	add := func(name string, required bool, err error, detail string) {
		item := preflightItem{Name: name, Required: required, OK: err == nil, Detail: detail}
		if err != nil {
			item.Detail = err.Error()
			if required {
				report.OK = false
			}
		}
		report.Items = append(report.Items, item)
	}
	tool := func(name string, required bool) {
		path, err := exec.LookPath(name)
		detail := path
		if err == nil {
			if version := toolVersion(name); version != "" {
				detail += " (" + version + ")"
			}
		}
		add(name, required, err, detail)
	}

	capture := func() {
		tool("just", true)
		tool("lua5.4", true)
		tool("cartesi-machine", true)
		checkDaveRoot(opts.daveRoot, add)
	}
	replay := func() {
		for _, bin := range []string{opts.nodeBin, opts.cliBin} {
			_, err := exec.LookPath(bin)
			add(filepath.Base(bin), true, err, bin)
		}
		checkPostgres(ctx, opts.dbAdmin, add)
		checkEnv(add)
	}
	sling := func() {
		if opts.slingBin == "" && opts.daveRoot != "" {
			opts.slingBin = filepath.Join(opts.daveRoot, slingBinaryPath)
		}
		if opts.slingBin == "" {
			add("Sling node", true, errors.New("set --sling-bin or DAVE_NODE_BIN"), "")
		} else if sum, _, err := sha256File(opts.slingBin); err != nil {
			add("Sling node", true, err, "")
		} else {
			add("Sling node", true, nil, fmt.Sprintf("%s sha256 %s", opts.slingBin, shortHash(sum)))
			checkLibraries(opts.slingBin, add)
		}
	}
	switch opts.capability {
	case "capture":
		tool("anvil", true)
		tool("cartesi-machine-stored-hash", false)
		capture()
	case "replay":
		tool("anvil", true)
		tool("cartesi-machine-stored-hash", false)
		replay()
	case "suite":
		tool("anvil", true)
		tool("cartesi-machine-stored-hash", false)
		capture()
		replay()
	case "run-sling":
		sling()
		checkRPC(ctx, opts.rpc, add)
	default:
		return withExitCode(exitUsage, fmt.Errorf("unknown capability %q", opts.capability))
	}

	if err := emit(cmd.OutOrStdout(), report); err != nil {
		return err
	}
	if !report.OK {
		return withExitCode(exitPrerequisites, errors.New("prerequisites are missing"))
	}
	return nil
}

type addFunc func(name string, required bool, err error, detail string)

func checkDaveRoot(root string, add addFunc) {
	if root == "" {
		add("DAVE_ROOT", true, errors.New("set --dave-root or DAVE_ROOT"), "")
		return
	}
	rev, dirty, err := gitRevision(root)
	detail := fmt.Sprintf("%s at %s", root, rev)
	if dirty {
		detail += " (modified)"
	}
	add("DAVE_ROOT", true, err, detail)
	for _, item := range []struct {
		name, path string
		required   bool
	}{
		{"Sling node build", slingBinaryPath, true},
		{"devnet bundle", "cartesi-rollups/contracts/state.json", true},
		{"deployments", deploymentsDir, true},
		{"echo image", filepath.Join(programsDir, "echo", "machine-image"), false},
		{"honeypot image", filepath.Join(programsDir, "honeypot", "machine-image"), false},
	} {
		_, err := os.Stat(filepath.Join(root, item.path))
		add(item.name, item.required, err, item.path)
	}
}

func checkPostgres(ctx context.Context, url string, add addFunc) {
	if url == "" {
		add("postgres", true, errors.New("set CARTESI_DATABASE_CONNECTION or --db-admin"), "")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	conn, err := pgx.Connect(ctx, url)
	if err == nil {
		defer conn.Close(ctx)
		var canCreate bool
		err = conn.QueryRow(ctx, "SELECT rolcreatedb OR rolsuper FROM pg_roles WHERE rolname = current_user").Scan(&canCreate)
		if err == nil && !canCreate {
			err = errors.New("the role cannot create databases")
		}
	}
	add("postgres", true, err, redactURL(url))
}

func checkRPC(ctx context.Context, url string, add addFunc) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	c, err := dialChain(ctx, url)
	if err != nil {
		add("rpc", true, err, url)
		return
	}
	defer c.close()
	chainID, err := c.chainID(ctx)
	if err != nil {
		add("rpc", true, err, url)
		return
	}
	header, err := c.head(ctx)
	if err != nil {
		add("rpc", true, err, url)
		return
	}
	add("rpc", true, nil, fmt.Sprintf("%s chain %d head %d", url, chainID, header.Number))
}

// checkEnv reports the node settings that replay inherits and does not set.
func checkEnv(add addFunc) {
	for _, key := range []string{"CARTESI_AUTH_MNEMONIC", envPrtMnemonic} {
		if os.Getenv(key) == "" {
			add(key, false, errors.New("unset; replay uses the test mnemonic on chain 31337"), "")
		} else {
			add(key, false, nil, "set")
		}
	}
}

// checkLibraries reports the dynamic libraries of the Sling executable. A
// missing library is reported, never installed.
func checkLibraries(bin string, add addFunc) {
	inspector, args := "otool", []string{"-L", bin}
	if _, err := exec.LookPath(inspector); err != nil {
		inspector, args = "ldd", []string{bin}
	}
	out, err := exec.Command(inspector, args...).CombinedOutput()
	if err != nil {
		add("libraries", false, fmt.Errorf("%s: %w", inspector, err), "")
		return
	}
	var missing []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "not found") {
			missing = append(missing, strings.TrimSpace(line))
		}
	}
	if len(missing) > 0 {
		add("libraries", true, fmt.Errorf("missing: %s", strings.Join(missing, "; ")), "")
		return
	}
	add("libraries", false, nil, fmt.Sprintf("%d lines from %s", len(strings.Split(strings.TrimSpace(string(out)), "\n")), inspector))
}
