// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func streamingModels() []any {
	return []any{&WithdrawalConfig{}, &Application{}, &ExecutionParameters{}, &Epoch{}, &Input{}, &Output{}, &Withdrawal{}, &Report{}, &Tournament{}, &Commitment{}, &Match{}, &MatchAdvanced{}, &StateHash{}, &TournamentSnapshot{}, &TournamentInnerResult{}, &CommitmentSnapshot{}, &MatchSnapshot{}, &MatchBisectionSnapshot{}, &TournamentCreationEvent{}, &LeafMatchSeal{}, &BondEvent{}, new(Uint256)}
}

func fillStreamingFixture(v reflect.Value) {
	if v.Type() == reflect.TypeFor[time.Time]() {
		v.Set(reflect.ValueOf(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fillStreamingFixture(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanSet() {
				fillStreamingFixture(v.Field(i))
			}
		}
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			fillStreamingFixture(v.Index(i))
		}
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 2, 2))
		for i := 0; i < v.Len(); i++ {
			fillStreamingFixture(v.Index(i))
		}
	case reflect.String:
		v.SetString("fixture<>&雪")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(17)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(17)
	}
}

func TestStreamingModelWireFormat(t *testing.T) {
	// Fixtures preserve the expected wire format, including the alpha.6 dispute fields.
	fixture := map[string]json.RawMessage{}
	data, err := os.ReadFile("testdata/streaming_wire.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &fixture))
	for _, value := range streamingModels() {
		name := reflect.TypeOf(value).Elem().Name()
		populated := reflect.New(reflect.TypeOf(value).Elem()).Interface()
		fillStreamingFixture(reflect.ValueOf(populated).Elem())
		for _, tc := range []struct {
			name  string
			value any
		}{{"zero", value}, {"populated", populated}} {
			key := name + "/" + tc.name
			t.Run(key, func(t *testing.T) {
				require.Implements(t, (*jsonv2.MarshalerTo)(nil), tc.value)
				require.Contains(t, fixture, key)
				data, err := jsonv2.Marshal(tc.value, json.DefaultOptionsV1())
				require.NoError(t, err)
				require.JSONEq(t, string(fixture[key]), string(data))
				legacy, err := json.Marshal(tc.value)
				require.NoError(t, err)
				require.Equal(t, string(data), string(legacy))
			})
		}
	}
}
