// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package services provides mechanisms to start multiple services in the
// background
package services

import (
	"context"
	"fmt"
)

type Service interface {
	fmt.Stringer

	// Starts a service and sends a message to the channel when ready
	Start(ctx context.Context, ready chan<- struct{}) error
}
