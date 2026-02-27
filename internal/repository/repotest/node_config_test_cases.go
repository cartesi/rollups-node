// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"encoding/json"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

type NodeConfigSuite struct {
	BaseSuite
}

func NewNodeConfigSuite(factory RepositoryFactory) *NodeConfigSuite {
	return &NodeConfigSuite{BaseSuite: BaseSuite{factory: factory}}
}

func (s *NodeConfigSuite) TestSaveAndLoadNodeConfigRaw() {
	s.Run("RoundTrip", func() {
		key := "test-config"
		value := map[string]string{"foo": "bar"}
		data, err := json.Marshal(value)
		s.Require().NoError(err)

		err = s.Repo.SaveNodeConfigRaw(s.Ctx, key, data)
		s.Require().NoError(err)

		got, createdAt, updatedAt, err := s.Repo.LoadNodeConfigRaw(s.Ctx, key)
		s.Require().NoError(err)

		// Compare JSON semantically (PostgreSQL may reformat whitespace)
		var expected, actual map[string]string
		s.Require().NoError(json.Unmarshal(data, &expected))
		s.Require().NoError(json.Unmarshal(got, &actual))
		s.Equal(expected, actual)

		s.False(createdAt.IsZero())
		s.False(updatedAt.IsZero())
	})

	s.Run("UpdateExistingKey", func() {
		key := "update-test"
		data1, _ := json.Marshal("value1")
		data2, _ := json.Marshal("value2")

		err := s.Repo.SaveNodeConfigRaw(s.Ctx, key, data1)
		s.Require().NoError(err)

		err = s.Repo.SaveNodeConfigRaw(s.Ctx, key, data2)
		s.Require().NoError(err)

		got, _, _, err := s.Repo.LoadNodeConfigRaw(s.Ctx, key)
		s.Require().NoError(err)

		// Compare JSON semantically
		var expected, actual string
		s.Require().NoError(json.Unmarshal(data2, &expected))
		s.Require().NoError(json.Unmarshal(got, &actual))
		s.Equal(expected, actual)
	})

	s.Run("NotFound", func() {
		_, _, _, err := s.Repo.LoadNodeConfigRaw(s.Ctx, "nonexistent-key")
		s.ErrorIs(err, repository.ErrNotFound)
	})
}

func (s *NodeConfigSuite) TestGenericNodeConfig() {
	s.Run("SaveAndLoadTyped", func() {
		type TestConfig struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}

		nc := &NodeConfig[TestConfig]{
			Key:   "typed-config",
			Value: TestConfig{Name: "test", Count: 42},
		}

		err := repository.SaveNodeConfig(s.Ctx, s.Repo, nc)
		s.Require().NoError(err)

		got, err := repository.LoadNodeConfig[TestConfig](s.Ctx, s.Repo, "typed-config")
		s.Require().NoError(err)
		s.Require().NotNil(got)
		s.Equal("test", got.Value.Name)
		s.Equal(42, got.Value.Count)
	})
}
