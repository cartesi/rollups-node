// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package httpclient

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const (
	maxPublicationRequestBytes = 1 << 20
	rpcFailedMessage           = "RPC request failed"
	httpFailedMessage          = "HTTP request failed"
	nonceLowMessage            = "nonce too low"
	alreadyKnownMessage        = "already known"
	publicationMethod          = "eth_sendRawTransaction"
)

func isPublicationRequest(req *http.Request) bool {
	if req.GetBody == nil {
		return false
	}
	body, err := req.GetBody()
	if err != nil {
		return false
	}
	data, readErr := io.ReadAll(io.LimitReader(body, maxPublicationRequestBytes+1))
	closeErr := body.Close()
	if readErr != nil || closeErr != nil || len(data) > maxPublicationRequestBytes {
		return false
	}
	var call struct {
		Method string `json:"method"`
	}
	return json.Unmarshal(data, &call) == nil && call.Method == publicationMethod
}

// PublicationMessage returns a fixed diagnostic category. Callers must only
// supply the message from an eth_sendRawTransaction error envelope.
func PublicationMessage(code int, message string) string {
	m := strings.ToLower(message)
	if code == 3 || strings.Contains(m, "execution reverted") {
		return rpcFailedMessage
	}
	if m == "known transaction" || strings.HasPrefix(m, "known transaction:") {
		return alreadyKnownMessage
	}
	for _, category := range []struct {
		message string
		phrases []string
	}{
		{alreadyKnownMessage, []string{alreadyKnownMessage, "transaction already imported"}},
		{nonceLowMessage, []string{nonceLowMessage}},
		{"replacement transaction underpriced", []string{"replacement transaction underpriced"}},
		{"transaction underpriced", []string{
			"transaction underpriced", "transaction gas price below minimum", "gas price below configured minimum gas price",
		}},
		{"max fee per gas less than block base fee", []string{
			"max fee per gas less than block base fee", "gas price below current base fee",
		}},
		{"insufficient funds", []string{"insufficient funds"}},
		{"intrinsic gas too low", []string{
			"exceeds block gas limit", "transaction gas limit too high", "intrinsic gas too low",
			"insufficient gas for floor data gas cost",
		}},
		{"invalid sender", []string{
			"max priority fee per gas higher than max fee per gas", "invalid sender",
			"only replay-protected (eip-155) transactions allowed over rpc",
		}},
		{"txpool is full", []string{
			"txpool is full", "future transaction tries to replace pending", "account limit exceeded", "out of capacity",
		}},
	} {
		for _, phrase := range category.phrases {
			if strings.Contains(m, phrase) {
				return category.message
			}
		}
	}
	if strings.Contains(m, "tx fee (") && strings.Contains(m, "exceeds the configured cap") {
		return "tx fee (redacted) exceeds the configured cap"
	}
	return rpcFailedMessage
}

func publicationResponse(body []byte, publication bool) []byte {
	var envelope struct {
		Error *struct {
			Code    *int   `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil || envelope.Error == nil || envelope.Error.Code == nil {
		return []byte(httpFailedMessage)
	}
	response := struct {
		JSONRPC string `json:"jsonrpc"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{JSONRPC: "2.0"}
	response.Error.Code = *envelope.Error.Code
	response.Error.Message = rpcFailedMessage
	if publication {
		response.Error.Message = PublicationMessage(*envelope.Error.Code, envelope.Error.Message)
	}
	data, err := json.Marshal(response)
	if err != nil {
		return []byte(httpFailedMessage)
	}
	return data
}
