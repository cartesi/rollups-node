// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package jsonutil defines the JSON wire conventions shared by node services,
// command-line tools, and their clients. Serialization uses encoding/json/v2.
package jsonutil

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
)

// Options preserves deterministic keys, null collections, case-insensitive
// decoding, and escaping used by the existing public JSON formats.
var Options = json.JoinOptions(
	json.Deterministic(true),
	json.FormatNilMapAsNull(true),
	json.FormatNilSliceAsNull(true),
	json.MatchCaseInsensitiveNames(true),
	jsontext.EscapeForHTML(true),
	jsontext.EscapeForJS(true),
	jsontext.AllowDuplicateNames(true),
	jsontext.AllowInvalidUTF8(true),
	jsontext.PreserveRawStrings(true),
)

// Indent appends formatted JSON to dst without decoding it into Go objects.
func Indent(dst *bytes.Buffer, src []byte, prefix, indent string) error {
	value := jsontext.Value(src).Clone()
	if err := value.Indent(Options, jsontext.EscapeForHTML(false), jsontext.EscapeForJS(false), jsontext.WithIndentPrefix(prefix), jsontext.WithIndent(indent)); err != nil {
		return err
	}
	_, err := dst.Write(value)
	return err
}
