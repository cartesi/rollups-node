// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inspectclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/cartesi/rollups-node/internal/httpclient"
	"github.com/cartesi/rollups-node/internal/inspectmessages"
)

// SafeClient renders errors from request construction and all request editors
// with public endpoint details. Constructor options configure the generated
// client before its transport is protected; it cannot be mutated afterward.
type SafeClient struct {
	client       *Client
	publicServer string
}

// Endpoint returns the public scheme and host for diagnostics.
func (c *SafeClient) Endpoint() string { return c.publicServer }

// InspectPostWithBody protects diagnostics from both persistent and per-call
// request editors as well as request construction and transport failures.
func (c *SafeClient) InspectPostWithBody(
	ctx context.Context, dapp, contentType string, body io.Reader, editors ...RequestEditorFn,
) (*http.Response, error) {
	response, err := c.client.InspectPostWithBody(ctx, dapp, contentType, body, editors...)
	if err != nil {
		return response, httpclient.Diagnostic(ctx, err, "inspect request", c.publicServer)
	}
	return response, nil
}

// NewSafeClient confines endpoint credentials to the HTTP transport. The
// generated client builds requests with a public scheme-and-host URL; the
// transport restores the configured base path, query and authentication.
// WithHTTPClient supports ordinary *http.Client values so their transport can
// be protected while preserving client settings.
func NewSafeClient(server string, opts ...ClientOption) (*SafeClient, error) {
	parsed, err := httpclient.Validate(server)
	if err != nil {
		return nil, err
	}
	publicServer := (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String()
	client := &Client{Server: server}
	for _, opt := range opts {
		if err := opt(client); err != nil {
			return nil, httpclient.Diagnostic(context.Background(), err, "configure inspect client", publicServer)
		}
	}
	var underlying *http.Client
	if client.Client != nil {
		var ok bool
		underlying, ok = client.Client.(*http.Client)
		if !ok {
			return nil, fmt.Errorf("inspect client at %s requires an *http.Client", publicServer)
		}
	}
	publicServer, protected, err := httpclient.NewClient(client.Server, underlying, httpclient.Options{
		AppendOperation: true,
		AllowedErrorMessages: []string{
			inspectmessages.MissingApplicationAddress, inspectmessages.MethodNotAllowed,
			inspectmessages.PayloadTooLarge, inspectmessages.BadRequest,
			inspectmessages.TerminalApplication, inspectmessages.MachineNotReady,
			inspectmessages.ForeclosedApplication, inspectmessages.ApplicationNotFound,
			inspectmessages.InspectAtCapacity,
		},
	})
	if err != nil {
		return nil, err
	}
	client.Server = publicServer + "/"
	client.Client = protected
	return &SafeClient{client: client, publicServer: publicServer}, nil
}
