package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace"
)

func TestBacktestGetDailyAccuracyReturnsDays(t *testing.T) {
	day := time.Date(2026, 2, 13, 0, 0, 0, 0, time.UTC)
	pool := &btStubPool{
		rowsData: [][]any{
			{"ml_logreg_up4h", day, 12, 9, 0.75},
		},
	}
	repo := NewBacktestRepository(pool, trace.NewNoopTracerProvider().Tracer("test"))

	results, err := repo.GetDailyAccuracy(context.Background(), "ml_logreg_up4h", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ModelKey != "ml_logreg_up4h" {
		t.Fatalf("expected ml_logreg_up4h, got %s", results[0].ModelKey)
	}
	if results[0].Total != 12 || results[0].Correct != 9 {
		t.Fatalf("expected 12/9, got %d/%d", results[0].Total, results[0].Correct)
	}
}

func TestBacktestGetDailyAccuracyEmpty(t *testing.T) {
	pool := &btStubPool{}
	repo := NewBacktestRepository(pool, trace.NewNoopTracerProvider().Tracer("test"))

	results, err := repo.GetDailyAccuracy(context.Background(), "ml_logreg_up4h", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestBacktestGetAccuracySummary(t *testing.T) {
	now := time.Now().UTC()
	pool := &btStubPool{
		rowsData: [][]any{
			{"ml_logreg_up4h", now, 100, 78, 0.78},
			{"ml_xgboost_up4h", now, 100, 85, 0.85},
		},
	}
	repo := NewBacktestRepository(pool, trace.NewNoopTracerProvider().Tracer("test"))

	results, err := repo.GetAccuracySummary(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestBacktestListRecentPredictionsDefaultLimit(t *testing.T) {
	pool := &btStubPool{}
	repo := NewBacktestRepository(pool, trace.NewNoopTracerProvider().Tracer("test"))

	results, err := repo.ListRecentPredictions(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

// --- stubs ---

type btStubPool struct {
	rowsData [][]any
}

func (s *btStubPool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (s *btStubPool) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return &btStubBatchResults{}
}

func (s *btStubPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if s.rowsData == nil {
		return &btStubRows{}, nil
	}
	dataCopy := make([][]any, len(s.rowsData))
	for i := range s.rowsData {
		row := make([]any, len(s.rowsData[i]))
		copy(row, s.rowsData[i])
		dataCopy[i] = row
	}
	return &btStubRows{data: dataCopy}, nil
}

func (s *btStubPool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return &btStubRow{}
}

type btStubBatchResults struct{}

func (btStubBatchResults) Exec() (pgconn.CommandTag, error) { return pgconn.CommandTag{}, nil }
func (btStubBatchResults) Query() (pgx.Rows, error)         { return &btStubRows{}, nil }
func (btStubBatchResults) QueryRow() pgx.Row                { return &btStubRow{} }
func (btStubBatchResults) Close() error                     { return nil }

type btStubRows struct {
	data [][]any
	idx  int
}

func (r *btStubRows) Close()                                       {}
func (r *btStubRows) Err() error                                   { return nil }
func (r *btStubRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *btStubRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *btStubRows) Next() bool {
	if len(r.data) == 0 || r.idx >= len(r.data) {
		return false
	}
	r.idx++
	return true
}

func (r *btStubRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.data) {
		return fmt.Errorf("invalid scan index")
	}
	row := r.data[r.idx-1]
	for i, d := range dest {
		switch ptr := d.(type) {
		case *int64:
			*ptr = row[i].(int64)
		case *int:
			*ptr = row[i].(int)
		case *int16:
			*ptr = int16(row[i].(int))
		case *string:
			*ptr = row[i].(string)
		case *float64:
			*ptr = row[i].(float64)
		case *bool:
			*ptr = row[i].(bool)
		case **bool:
			if row[i] == nil {
				*ptr = nil
			} else {
				v := row[i].(bool)
				*ptr = &v
			}
		case **int64:
			if row[i] == nil {
				*ptr = nil
			} else {
				v := row[i].(int64)
				*ptr = &v
			}
		case **float64:
			if row[i] == nil {
				*ptr = nil
			} else {
				v := row[i].(float64)
				*ptr = &v
			}
		case **time.Time:
			if row[i] == nil {
				*ptr = nil
			} else {
				v := row[i].(time.Time)
				*ptr = &v
			}
		case *time.Time:
			*ptr = row[i].(time.Time)
		default:
			return fmt.Errorf("unsupported dest type %T", d)
		}
	}
	return nil
}

func (r *btStubRows) Values() ([]any, error) { return nil, nil }
func (r *btStubRows) RawValues() [][]byte    { return nil }
func (r *btStubRows) Conn() *pgx.Conn        { return nil }

type btStubRow struct{}

func (btStubRow) Scan(dest ...any) error { return nil }
