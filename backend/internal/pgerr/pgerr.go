package pgerr

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

func IsUnique(err error) bool     { return hasCode(err, "23505") }
func IsForeignKey(err error) bool { return hasCode(err, "23503") }

func hasCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
