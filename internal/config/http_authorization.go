// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/spf13/viper"
	"golang.org/x/net/http/httpguts"
)

var (
	errHTTPAuthorization = errors.New("malformed BlockchainHttpAuthorization, expected <key>:<value> with a valid HTTP header name and value")
)

// HTTPAuthorizationOption unwraps the http authorization config `key:val`
// into a ClientOption that rpc accepts.
//
// Authorization is optional when neither the direct value nor its _FILE
// counterpart is configured. Once configured, invalid input is an error so the
// service refuses to start instead of silently sending requests without the
// header.
func HTTPAuthorizationOption() (rpc.ClientOption, error) {
	if viper.GetString(BLOCKCHAIN_HTTP_AUTHORIZATION) == "" &&
		viper.GetString(BLOCKCHAIN_HTTP_AUTHORIZATION_FILE) == "" {
		return nil, nil
	}

	auth, err := GetBlockchainHttpAuthorization()
	if err != nil {
		return nil, err
	}

	key, value, err := parseHTTPAuthorization(auth.Value)
	if err != nil {
		return nil, err
	}

	return rpc.WithHTTPAuth(func(h http.Header) error {
		h.Set(key, value)
		return nil
	}), nil
}

func parseHTTPAuthorization(value string) (string, string, error) {
	key, val, ok := strings.Cut(value, ":")
	if !ok {
		return "", "", errHTTPAuthorization
	}

	key = strings.TrimSpace(key)
	val = strings.TrimSpace(val)
	if !httpguts.ValidHeaderFieldName(key) ||
		val == "" ||
		!httpguts.ValidHeaderFieldValue(val) {
		return "", "", errHTTPAuthorization
	}

	return key, val, nil
}
