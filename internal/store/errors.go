package store

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

type NotFoundError struct {
	Resource string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found", e.Resource)
}

type AlreadyExistsError struct {
	Resource string
}

func (e *AlreadyExistsError) Error() string {
	return fmt.Sprintf("%s already exists", e.Resource)
}

type ForeignKeyViolationError struct {
	Resource string
}

func (e *ForeignKeyViolationError) Error() string {
	return fmt.Sprintf("%s violates foreign key constraint", e.Resource)
}

func NotFound(resource string) error {
	return &NotFoundError{Resource: resource}
}

func AlreadyExists(resource string) error {
	return &AlreadyExistsError{Resource: resource}
}

func ForeignKeyViolation(resource string) error {
	return &ForeignKeyViolationError{Resource: resource}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23503"
}
