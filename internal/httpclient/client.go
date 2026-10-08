// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package httpclient validates HTTP endpoint configuration.
package httpclient

import (
	"errors"
	"net/url"
)

// Validate accepts HTTP(S) endpoints with a DNS name or IP address. Its errors never
// include the input, including failures from net/url.
func Validate(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("invalid HTTP endpoint")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, errors.New("HTTP endpoint requires http or https and a DNS name or IP address")
	}
	return u, nil
}
