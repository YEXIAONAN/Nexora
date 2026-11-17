package database

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"regexp"
	"syscall"

	"github.com/jackc/pgx/v5/pgconn"
)

var sqlState = regexp.MustCompile(`^[0-9A-Z]{5}$`)

type diagnosticError struct {
	message string
	cause   error
}

func (e *diagnosticError) Error() string { return e.message }
func (e *diagnosticError) Unwrap() error { return e.cause }

// SafeError keeps error classification and the original cause for errors.Is/As,
// but never renders driver text, connection strings or server-supplied details.
// Operation names and a redacted endpoint are logged separately by callers.
func SafeError(err error) error {
	if err == nil {
		return nil
	}
	var parseErr *pgconn.ParseConfigError
	var pgErr *pgconn.PgError
	var dnsErr *net.DNSError
	var tlsErr *tls.CertificateVerificationError
	var netErr net.Error
	message := "database operation failed"
	switch {
	case errors.Is(err, context.Canceled):
		message = "database operation canceled"
	case errors.Is(err, context.DeadlineExceeded):
		message = "database operation timed out"
	case errors.As(err, &parseErr):
		message = "invalid database connection configuration; check URL and connection options"
	case errors.As(err, &pgErr) && sqlState.MatchString(pgErr.Code):
		message = "PostgreSQL error (SQLSTATE " + pgErr.Code + ")"
	case errors.As(err, &dnsErr):
		message = "database hostname lookup failed"
	case errors.As(err, &tlsErr):
		message = "database TLS certificate verification failed"
	case errors.Is(err, syscall.ECONNREFUSED):
		message = "database connection refused"
	case errors.As(err, &netErr) && netErr.Timeout():
		message = "database connection timed out"
	}
	return &diagnosticError{message: message, cause: err}
}
