// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgconn"
)

// Driver error strings can carry hostnames, usernames, database names or DSN
// values. Only typed causes and a validated SQLSTATE may enter diagnostics.
func safePostgresConnectCause(err error) string {
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "connection timeout"
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		if pgError.Code == "28P01" || pgError.Code == "28000" {
			return "authentication refused"
		}
		if len(pgError.Code) == 5 && strings.Trim(pgError.Code, "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ") == "" {
			return "Postgres refused connection (SQLSTATE " + pgError.Code + ")"
		}
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "DNS resolution failed"
	}
	var certificate *tls.CertificateVerificationError
	var authority x509.UnknownAuthorityError
	if errors.As(err, &certificate) || errors.As(err, &authority) {
		return "TLS certificate verification failed"
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return "network timeout"
	}
	for _, cause := range []struct {
		err  error
		name string
	}{
		{syscall.ECONNREFUSED, "connection refused"}, {syscall.ECONNRESET, "connection reset"},
		{syscall.ENETUNREACH, "network unreachable"}, {syscall.EHOSTUNREACH, "host unreachable"},
	} {
		if errors.Is(err, cause.err) {
			return cause.name
		}
	}
	return "connection failed; inspect database and network health"
}
