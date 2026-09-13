// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type nodeConfigReadStub struct {
	NodeConfigRepository
	load func(context.Context, string) ([]byte, time.Time, time.Time, error)
}

func (r nodeConfigReadStub) LoadNodeConfigRaw(ctx context.Context, key string) ([]byte, time.Time, time.Time, error) {
	return r.load(ctx, key)
}

func TestLoadNodeConfigResult(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "nil payload", want: "has no JSON value"},
		{name: "empty payload", raw: []byte{}, want: "unmarshal node_config value failed"},
		{name: "invalid JSON", raw: []byte(`{`), want: "unmarshal node_config value failed"},
		{name: "valid payload", raw: []byte(`{"ChainID":42}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			createdAt := time.Unix(1, 0)
			updatedAt := createdAt.Add(time.Second)
			repo := nodeConfigReadStub{load: func(ctx context.Context, key string) ([]byte, time.Time, time.Time, error) {
				require.Equal(t, t.Context(), ctx)
				require.Equal(t, "test-config", key)
				return test.raw, createdAt, updatedAt, nil
			}}
			type testConfig struct{ ChainID uint64 }
			got, err := LoadNodeConfig[testConfig](t.Context(), repo, "test-config")
			if test.want != "" {
				require.ErrorContains(t, err, test.want)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got, "a successful load must return a record")
			require.Equal(t, "test-config", got.Key)
			require.Equal(t, testConfig{ChainID: 42}, got.Value)
			require.Equal(t, createdAt, got.CreatedAt)
			require.Equal(t, updatedAt, got.UpdatedAt)
		})
	}
}
