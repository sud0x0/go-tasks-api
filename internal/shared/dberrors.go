package shared

import (
	"database/sql/driver"
	"errors"
	"net"

	"github.com/jackc/pgx/v5/pgconn"
)

// IsDBUnavailable reports whether err (or any error in its chain) indicates
// that the database is unreachable, as opposed to a query-level failure
// (missing table, permission denied, constraint violation, deadlock, etc.).
//
// Handlers use this to distinguish "service temporarily unavailable" (503)
// from "internal error" (500) when mapping a domain ErrDatabase to HTTP.
// The distinction matters for operators: 503 says "the database is down or
// unreachable, check connectivity"; 500 says "the API talked to the database
// but the query failed, check the logged actual_error for the cause".
func IsDBUnavailable(err error) bool {
	if err == nil {
		return false
	}

	// pgx surfaces dial/handshake failures as *pgconn.ConnectError.
	var connErr *pgconn.ConnectError
	if errors.As(err, &connErr) {
		return true
	}

	// driver.ErrBadConn is returned when a pooled connection is broken.
	if errors.Is(err, driver.ErrBadConn) {
		return true
	}

	// Generic network errors (DNS lookup failure, connection refused, etc.).
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// A *pgconn.PgError means the server responded — query-level problem,
	// not unavailability. Don't treat these as unavailable even if some
	// other check above might have matched.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return false
	}

	return false
}
