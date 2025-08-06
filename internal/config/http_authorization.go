// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/rpc"
)

// Unwrap the http authorization config `key:val` into a ClientOption that rpc accepts
func HTTPAuthorizationOption() (rpc.ClientOption, error) {

	// no authorization is allowed.
	auth, err := GetBlockchainHttpAuthorization()
	if err != nil {
		return nil, nil
	}

	kv := strings.SplitN(auth.Value, ":", 2)
	if len(kv) != 2 {
		return nil, fmt.Errorf("malformed BlockchainHttpAuthorization, expected <key>:<value>")
	}
	key := strings.TrimSpace(kv[0])
	val := strings.TrimSpace(kv[1])

	return rpc.WithHTTPAuth(func(h http.Header) error {
		h.Set(key, val)
		return nil
	}), nil
}
