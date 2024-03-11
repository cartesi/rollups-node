// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"os"
	"sort"

	"github.com/BurntSushi/toml"
)

func readTOML(name string) string {
	bytes, err := os.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

type configTOML = map[string](map[string]*Env)

func decodeTOML(data string) configTOML {
	var config configTOML
	_, err := toml.Decode(data, &config)
	if err != nil {
		panic(err)
	}
	return config
}

// Creates sorted lists of environment variables from the config
// to make the generated files deterministic.
func sortConfig(config configTOML) []Env {
	var topics []string
	mapping := make(map[string]([]string)) // topic names to env names

	for name, topic := range config {
		var envs []string
		for name, env := range topic {
			env.Name = name // initializes the environment variable's name
			envs = append(envs, name)
		}
		sort.Strings(envs)

		topics = append(topics, name)
		mapping[name] = envs
	}
	sort.Strings(topics)

	var envs []Env
	for _, topic := range topics {
		for _, name := range mapping[topic] {
			envs = append(envs, *config[topic][name])
		}
	}

	return envs
}
