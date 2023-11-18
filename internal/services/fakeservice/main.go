// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This file creates a dummy webserver with the sole pupose of being used
// as a binary to test the services.Service struct
package main

import (
	"net/http"
	"os"
)

func main() {
	addr := os.Getenv("SERVICE_ADDRESS")
	err := http.ListenAndServe(addr, nil)
	panic(err)
}
