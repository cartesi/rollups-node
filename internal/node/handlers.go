// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/cartesi/rollups-node/internal/node/config"
)

func newHttpServiceHandler(c config.NodeConfig) http.Handler {
	handler := http.NewServeMux()
	handler.Handle("/healthz", http.HandlerFunc(healthcheckHandler))

	graphqlProxy := newReverseProxy(c.HttpAddress, getPort(c, portOffsetGraphQLServer))
	handler.Handle("/graphql", graphqlProxy)

	dispatcherProxy := newReverseProxy(c.HttpAddress, getPort(c, portOffsetDispatcher))
	handler.Handle("/metrics", dispatcherProxy)

	inspectProxy := newReverseProxy(c.HttpAddress, getPort(c, portOffsetInspectServer))
	handler.Handle("/inspect", inspectProxy)
	handler.Handle("/inspect/", inspectProxy)

	if c.FeatureHostMode {
		hostProxy := newReverseProxy(c.HttpAddress, getPort(c, portOffsetHostRunnerRollups))
		handler.Handle("/rollup/", http.StripPrefix("/rollup", hostProxy))
	}
	return handler
}

func healthcheckHandler(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Node received a healthcheck request")
	w.WriteHeader(http.StatusOK)
}

func newReverseProxy(address string, port int) *httputil.ReverseProxy {
	urlStr := fmt.Sprintf("http://%v:%v/", address, port)
	url, err := url.Parse(urlStr)
	if err != nil {
		panic(fmt.Sprintf("failed to parse url: %v", err))
	}
	proxy := httputil.NewSingleHostReverseProxy(url)
	return proxy
}
