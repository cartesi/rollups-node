// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/spf13/viper"
)

// HTTPAuthorizationOption unwraps the http authorization config `key:val`
// into a ClientOption that rpc accepts.
//
// Authorization is optional: when neither the direct value nor its _FILE
// counterpart is set it returns (nil, nil). When either is set but invalid
// (missing or non-conformant file, malformed value) the error is returned so
// the service refuses to start, as documented in docs/secrets.md.
func HTTPAuthorizationOption() (rpc.ClientOption, error) {
	if viper.GetString(BLOCKCHAIN_HTTP_AUTHORIZATION) == "" &&
		viper.GetString(BLOCKCHAIN_HTTP_AUTHORIZATION_FILE) == "" {
		return nil, nil
	}

	auth, err := GetBlockchainHttpAuthorization()
	if err != nil {
		return nil, err
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
