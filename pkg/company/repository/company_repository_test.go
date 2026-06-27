package company_repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type dryRunConnPool struct{}

func (dryRunConnPool) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, errors.New("unexpected database call in dry-run test")
}

func (dryRunConnPool) ExecContext(context.Context, string, ...interface{}) (sql.Result, error) {
	return nil, errors.New("unexpected database call in dry-run test")
}

func (dryRunConnPool) QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error) {
	return nil, errors.New("unexpected database call in dry-run test")
}

func (dryRunConnPool) QueryRowContext(context.Context, string, ...interface{}) *sql.Row {
	return &sql.Row{}
}

type queryCaptureLogger struct {
	statements []string
}

func (l *queryCaptureLogger) LogMode(logger.LogLevel) logger.Interface      { return l }
func (l *queryCaptureLogger) Info(context.Context, string, ...interface{})  {}
func (l *queryCaptureLogger) Warn(context.Context, string, ...interface{})  {}
func (l *queryCaptureLogger) Error(context.Context, string, ...interface{}) {}

func (l *queryCaptureLogger) Trace(_ context.Context, _ time.Time, sql func() (string, int64), _ error) {
	statement, _ := sql()
	l.statements = append(l.statements, statement)
}

func TestBackfillInstancesDoesNotCompareUUIDWithEmptyString(t *testing.T) {
	capture := &queryCaptureLogger{}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:             dryRunConnPool{},
		WithoutReturning: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
		Logger:               capture,
	})
	if err != nil {
		t.Fatalf("create dry-run database: %v", err)
	}

	repository := NewCompanyRepository(db)
	if err := repository.BackfillInstances("f4d82f4d-7d6b-4ad6-a723-f0c13527095f"); err != nil {
		t.Fatalf("backfill instances: %v", err)
	}

	statement := strings.Join(capture.statements, "\n")
	if !strings.Contains(statement, "company_id IS NULL") {
		t.Fatalf("expected NULL company filter, got SQL: %s", statement)
	}
	if strings.Contains(statement, "company_id = ''") {
		t.Fatalf("UUID column must not be compared with an empty string, got SQL: %s", statement)
	}
}
