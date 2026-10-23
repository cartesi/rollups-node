// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

// NodeConfigInitializer provisions configuration fixtures without replacing
// an existing value. Production services use the merging registration methods.
type NodeConfigInitializer interface {
	repository.NodeConfigReader
	InitializeNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error
}

// NodeConfigWriter permits whole-value replacement for configuration fixtures.
// Production repository interfaces expose reads and dedicated merging methods.
type NodeConfigWriter interface {
	SaveNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error
}

// SaveNodeConfig writes a typed configuration fixture.
func SaveNodeConfig[T any](
	ctx context.Context,
	repo NodeConfigWriter,
	nc *model.NodeConfig[T],
) error {
	data, err := json.Marshal(nc.Value)
	if err != nil {
		return fmt.Errorf("marshal node_config value failed: %w", err)
	}
	return repo.SaveNodeConfigRaw(ctx, nc.Key, data)
}

// InitializeNodeConfig saves only a missing fixture key, then returns the stored
// value. Callers must validate it: a concurrent initializer may have won with a
// different value.
func InitializeNodeConfig[T any](
	ctx context.Context, repo NodeConfigInitializer, nc *model.NodeConfig[T],
) (*model.NodeConfig[T], error) {
	data, err := json.Marshal(nc.Value)
	if err != nil {
		return nil, fmt.Errorf("marshal initial node_config value: %w", err)
	}
	if err := repo.InitializeNodeConfigRaw(ctx, nc.Key, data); err != nil {
		return nil, fmt.Errorf("initialize node_config %q: %w", nc.Key, err)
	}
	return repository.LoadNodeConfig[T](ctx, repo, nc.Key)
}
