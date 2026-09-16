// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"encoding/json"
	"fmt"
	"math/big"
	"slices"

	"github.com/cartesi/rollups-node/internal/model"
)

// PersistentChainConfig identifies the chain and the observation policy used to
// write service state. A restart must not silently change either value.
type PersistentChainConfig struct {
	DefaultBlock model.DefaultBlock
	ChainID      uint64
}

func (c PersistentChainConfig) Validate() error {
	if c.ChainID == 0 {
		return fmt.Errorf("ChainID must be greater than zero")
	}
	if !slices.Contains(model.DefaultBlockAllValues, c.DefaultBlock) {
		return fmt.Errorf("invalid DefaultBlock %q", c.DefaultBlock)
	}
	return nil
}

// UnmarshalJSON checks required fields. Plain struct decoding would accept a
// missing field or null as a valid zero value, including a disabled submitter.
func (c *PersistentChainConfig) UnmarshalJSON(data []byte) error {
	var raw struct {
		DefaultBlock *model.DefaultBlock
		ChainID      *uint64
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.DefaultBlock == nil || raw.ChainID == nil {
		return fmt.Errorf("persistent config requires non-null DefaultBlock and ChainID")
	}
	next := PersistentChainConfig{DefaultBlock: *raw.DefaultBlock, ChainID: *raw.ChainID}
	if err := next.Validate(); err != nil {
		return err
	}
	*c = next
	return nil
}

// CheckRequested rejects changes to a database's chain or observation policy.
// A stronger block policy does not repair data already read at a weaker policy.
func (c PersistentChainConfig) CheckRequested(requested PersistentChainConfig) error {
	if err := requested.Validate(); err != nil {
		return fmt.Errorf("invalid requested config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return fmt.Errorf("invalid saved config: %w", err)
	}
	if c.ChainID != requested.ChainID {
		return fmt.Errorf("chain ID mismatch: database=%d, configured=%d; saved config was not changed",
			c.ChainID, requested.ChainID)
	}
	if c.DefaultBlock != requested.DefaultBlock {
		return fmt.Errorf("observation policy mismatch: database=%s, configured=%s; saved config was not changed",
			c.DefaultBlock, requested.DefaultBlock)
	}
	return nil
}

// PersistentSubmitterConfig adds a submission mode fixed when the service first
// starts. Mode transitions require claim and tournament reconciliation that is
// not supported yet. In particular, observing a Quorum claim is not a local vote.
type PersistentSubmitterConfig struct {
	DefaultBlock           model.DefaultBlock
	ClaimSubmissionEnabled bool
	ChainID                uint64
}

func (c PersistentSubmitterConfig) chainConfig() PersistentChainConfig {
	return PersistentChainConfig{DefaultBlock: c.DefaultBlock, ChainID: c.ChainID}
}

func (c PersistentSubmitterConfig) Validate() error {
	return c.chainConfig().Validate()
}

func (c *PersistentSubmitterConfig) UnmarshalJSON(data []byte) error {
	var chain PersistentChainConfig
	if err := json.Unmarshal(data, &chain); err != nil {
		return err
	}
	var raw struct{ ClaimSubmissionEnabled *bool }
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.ClaimSubmissionEnabled == nil {
		return fmt.Errorf("persistent config requires non-null ClaimSubmissionEnabled")
	}
	*c = PersistentSubmitterConfig{
		DefaultBlock: chain.DefaultBlock, ChainID: chain.ChainID,
		ClaimSubmissionEnabled: *raw.ClaimSubmissionEnabled,
	}
	return nil
}

func (c PersistentSubmitterConfig) CheckRequested(requested PersistentSubmitterConfig) error {
	if err := c.chainConfig().CheckRequested(requested.chainConfig()); err != nil {
		return err
	}
	if c.ClaimSubmissionEnabled != requested.ClaimSubmissionEnabled {
		return fmt.Errorf("claim submission mode mismatch: database=%t, configured=%t; "+
			"mode changes on an existing database are not supported; saved config was not changed",
			c.ClaimSubmissionEnabled, requested.ClaimSubmissionEnabled)
	}
	return nil
}

// CheckNetworkChainID avoids truncating an invalid or oversized RPC chain ID.
func CheckNetworkChainID(network *big.Int, configured uint64) error {
	if network == nil || network.Sign() <= 0 || !network.IsUint64() {
		return fmt.Errorf("invalid network chain ID %v", network)
	}
	if network.Uint64() != configured {
		return fmt.Errorf("chain ID mismatch: network=%s, configured=%d", network, configured)
	}
	return nil
}
