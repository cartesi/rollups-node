// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"strconv"

	"github.com/cartesi/rollups-node/internal/services"
)

func newAnvilService(anvilStateFilePath string) services.CommandService {
	var s services.CommandService
	s.Name = "anvil"
	s.HealthcheckPort, _ = strconv.Atoi(ANVIL_HTTP_PORT)
	s.Path = "anvil"

	s.Args = append(s.Args, "--host", ANVIL_IP_ADDR)
	s.Args = append(s.Args, "--dump-state", anvilStateFilePath)
	s.Args = append(s.Args, "--state-interval", "1")
	s.Args = append(s.Args, "--silent")

	return s
}
