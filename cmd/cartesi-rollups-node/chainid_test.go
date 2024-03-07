// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ValidateChainIdSuite struct {
	suite.Suite
}

func TestValidateChainId(t *testing.T) {
	suite.Run(t, new(ValidateChainIdSuite))
}

func (s *ValidateChainIdSuite) TestItFailsIfChainIdsDoNotMatch() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"jsonrpc":"2.0","id":67,"result":"0x7a69"}`)
	}))
	defer ts.Close()
	localChainId := uint64(11111)

	err := validateChainId(context.Background(), localChainId, ts.URL)

	s.NotNil(err)
}

func (s *ValidateChainIdSuite) TestItReturnsNilIfChainIdsMatch() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"jsonrpc":"2.0","id":67,"result":"0x7a69"}`)
	}))
	defer ts.Close()
	localChainId := uint64(31337)

	err := validateChainId(context.Background(), localChainId, ts.URL)

	s.Nil(err)
}
