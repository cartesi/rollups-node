// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/cartesi/rollups-node/internal/httpclient"
)

// nullErrorTransport removes a null "error" member from JSON-RPC responses.
// JSON-RPC 2.0 forbids that member in a successful response, but some providers
// send it as null, and go-ethereum reports any error member as a failure since
// v1.17.5.
type nullErrorTransport struct {
	next http.RoundTripper
}

func (t nullErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		endpoint := (&url.URL{Scheme: req.URL.Scheme, Host: req.URL.Host}).String()
		return nil, httpclient.Diagnostic(req.Context(), err, "read RPC response", endpoint)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := validateRPCEnvelope(body); err != nil {
			endpoint := (&url.URL{Scheme: req.URL.Scheme, Host: req.URL.Host}).String()
			return nil, httpclient.Diagnostic(req.Context(), err, "decode RPC response", endpoint)
		}
	}
	if fixed, changed := dropNullError(body); changed {
		body = fixed
		resp.ContentLength = int64(len(body))
		resp.Header.Del("Content-Length")
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

// Validate the first JSON value and the same outer field types as go-ethereum
// before its decoder can expose malformed provider input in a diagnostic.
// Error, Result, ID and Params have no envelope type constraints and are skipped:
// successful consensus input is neither decoded nor rewritten.
func validateRPCEnvelope(body []byte) error {
	type envelope struct {
		Version string `json:"jsonrpc"`
		Method  string `json:"method"`
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var batch []envelope
		return json.NewDecoder(bytes.NewReader(body)).Decode(&batch)
	}
	var message envelope
	return json.NewDecoder(bytes.NewReader(body)).Decode(&message)
}

var errorMemberName = []byte(`"error"`)

// dropNullError removes a null "error" member from a JSON-RPC response or from
// each response of a batch. It reports whether it changed the body, and it leaves
// a body that is not a JSON object or array of objects alone.
func dropNullError(body []byte) ([]byte, bool) {
	if !bytes.Contains(body, errorMemberName) {
		return body, false
	}
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var batch []map[string]json.RawMessage
		if json.Unmarshal(body, &batch) != nil {
			return body, false
		}
		changed := false
		for _, msg := range batch {
			changed = dropNullErrorMember(msg) || changed
		}
		return marshalIfChanged(body, batch, changed)
	}
	var msg map[string]json.RawMessage
	if json.Unmarshal(body, &msg) != nil {
		return body, false
	}
	return marshalIfChanged(body, msg, dropNullErrorMember(msg))
}

func dropNullErrorMember(msg map[string]json.RawMessage) bool {
	value, ok := msg["error"]
	if !ok || !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return false
	}
	delete(msg, "error")
	return true
}

func marshalIfChanged(body []byte, value any, changed bool) ([]byte, bool) {
	if !changed {
		return body, false
	}
	fixed, err := json.Marshal(value)
	if err != nil {
		return body, false
	}
	return fixed, true
}
