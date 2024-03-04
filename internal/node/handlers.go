// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/cartesi/rollups-node/internal/config"
)

func newHttpServiceHandler(nodeConfig config.NodeConfig) http.Handler {
	handler := http.NewServeMux()
	handler.Handle("/healthz", http.HandlerFunc(healthcheckHandler))

	graphqlProxy, err := newReverseProxy(
		nodeConfig.CartesiHttpAddress(),
		getPort(nodeConfig.CartesiHttpPort(), portOffsetGraphQLServer),
	)
	if err != nil {
		config.ErrorLogger.Fatal(err)
	}
	handler.Handle("/graphql", graphqlProxy)

	dispatcherProxy, err := newReverseProxy(
		nodeConfig.CartesiHttpAddress(),
		getPort(nodeConfig.CartesiHttpPort(), portOffsetDispatcher),
	)
	if err != nil {
		config.ErrorLogger.Fatal(err)
	}
	handler.Handle("/metrics", dispatcherProxy)

	inspectProxy, err := newReverseProxy(
		nodeConfig.CartesiHttpAddress(),
		getPort(nodeConfig.CartesiHttpPort(), portOffsetInspectServer),
	)
	if err != nil {
		config.ErrorLogger.Fatal(err)
	}
	handler.Handle("/inspect", inspectProxy)
	handler.Handle("/inspect/", inspectProxy)

	if nodeConfig.CartesiFeatureHostMode() {
		hostProxy, err := newReverseProxy(
			nodeConfig.CartesiHttpAddress(),
			getPort(nodeConfig.CartesiHttpPort(), portOffsetHostRunnerRollups),
		)
		if err != nil {
			config.ErrorLogger.Fatal(err)
		}
		handler.Handle("/rollup/", http.StripPrefix("/rollup", hostProxy))
	}
	return handler
}

func healthcheckHandler(w http.ResponseWriter, r *http.Request) {
	config.DebugLogger.Println("received healthcheck request")
	w.WriteHeader(http.StatusOK)
}

func newReverseProxy(httpAddress string, port int) (*httputil.ReverseProxy, error) {
	urlStr := fmt.Sprintf(
		"http://%v:%v/",
		httpAddress,
		port,
	)

	url, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.ErrorLog = config.ErrorLogger
	return proxy, nil
}
