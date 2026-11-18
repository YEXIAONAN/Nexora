package contributions

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
)

// Keep only the SQLSTATE for retry classification and deadlock assertions.
// Query text, PostgreSQL DETAIL and credential-bearing parameters are discarded.
type stateError string

func (e stateError) Error() string    { return "contribution database conflict" }
func (e stateError) SQLState() string { return string(e) }

// Never attach a PostgreSQL message, DETAIL, query or parameter to a public
// error or contribution log. Receipt queries contain a transient credential.
func safeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.NotFound("the requested scoped resource does not exist")
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "40001", "40P01", "23505", "23514":
			return httpx.Conflict("the contribution state, evidence or version changed; reload before retrying").WithCause(stateError(pg.Code))
		case "23503":
			return httpx.NotFound("the requested scoped resource does not exist")
		case "22P02", "22001", "22003", "23502":
			return httpx.ValidationFailed(map[string]string{"request": "contains an invalid contribution value"})
		default:
			return httpx.Internal(fmt.Errorf("contribution database operation failed (SQLSTATE %s)", pg.Code))
		}
	}
	var api *httpx.APIError
	if errors.As(err, &api) && api.Status < 500 {
		return api
	}
	return httpx.Internal(errors.New("contribution operation failed"))
}
