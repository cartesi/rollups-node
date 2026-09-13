// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"fmt"
	"slices"
)

// scanEnum accepts the database representations supported by every string enum.
// An invalid value leaves the receiver unchanged.
func scanEnum[T ~string](dst *T, value any, values []T, name string) error {
	// Keep the existing public error text, including these two naming exceptions.
	prefix := "invalid value"
	if name == "SnapshotPolicy" {
		prefix = "invalid scan value"
	}
	typeName := name
	if name == "Consensus" {
		typeName = "ConsensusType"
	}
	var text string
	switch value := value.(type) {
	case string:
		text = value
	case []byte:
		text = string(value)
	default:
		return fmt.Errorf("%s for %s enum. Enum value has to be of type string or []byte", prefix, typeName)
	}
	parsed := T(text)
	if !slices.Contains(values, parsed) {
		return fmt.Errorf("%s '%s' for %s enum", prefix, text, name)
	}
	*dst = parsed
	return nil
}
