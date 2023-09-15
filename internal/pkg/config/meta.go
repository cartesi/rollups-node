// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
)

// Describes a config variable.
// This is useful for automatically generating documentation.
type MetaConfig struct {
	key          string
	description  string
	defaultValue string
	parse        func(string) error
}

func (c MetaConfig) getKey() string {
	return Prefix + c.key
}

// Meta config for each config variable
var configs []MetaConfig = []MetaConfig{
	{
		"PRINT_CONFIG",
		"If 'true' prints the config variables as they are parsed.\n" +
			"Can be 'true' or 'false'.",
		"true",
		func(value string) error {
			var err error
			PrintConfig, err = parseBool(value)
			return err
		},
	},
	{
		"LOG_LEVEL",
		"Sets the log level of the node, can be 'trace', 'info', or 'warning'.",
		"info",
		func(value string) error {
			valid := []string{"trace", "info", "warning"}
			if !slices.Contains(valid, value) {
				return errors.New("invalid value")
			}
			LogLevel = value
			return nil
		},
	},
}

func loadFromEnv(writer io.Writer) error {
	for _, c := range configs {
		value, exists := os.LookupEnv(c.getKey())
		if !exists {
			value = c.defaultValue
		}
		err := c.parse(value)
		if err != nil {
			msg := "failed to parse %v=%v because of %v"
			return errors.New(fmt.Sprintf(msg, c.getKey(), value, err))
		}
		if PrintConfig {
			fmt.Fprintf(writer, "%v=%v\n", c.getKey(), value)
		}
	}
	return nil
}

func genDocumentation(writer io.Writer) {
	for _, c := range configs {
		msg := "## %v\n%v\nDefault value: '%v'\n\n"
		fmt.Fprintf(writer, msg, c.getKey(), c.description, c.defaultValue)
	}
}
