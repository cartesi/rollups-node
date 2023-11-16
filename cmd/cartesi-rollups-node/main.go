// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"os"

	"github.com/cartesi/rollups-node/internal/logger"
)

func main() {
	logLevel := os.Getenv("CARTESI_LOG_LEVEL")
	_, enableTimestamp := os.LookupEnv("CARTESI_LOG_ENABLE_TIMESTAMP")
	logger.Init(logLevel, enableTimestamp)

	ctx := context.Background()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		logger.Error.Panic(err)
	}
}
