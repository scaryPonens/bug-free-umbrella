package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"bug-free-umbrella/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace"
)

func TestSignalImageRepositoryUpsertReady(t *testing.T) {
	pool := &imageRepoStubPool{
		queryRowValues: []any{int64(7), "image/png", int32(640), int32(480), time.Now().UTC().Add(time.Hour)},
	}
	repo := NewSignalImageRepository(pool, trace.NewNoopTracerProvider().Tracer("test"))

	ref, err := repo.UpsertSignalImageReady(context.Background(), 11, []byte{1, 2, 3}, "image/png", 640, 480, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref == nil || ref.ImageID != 7 || ref.MimeType != "image/png" {
		t.Fatalf("unexpected image ref: %+v", ref)
	}
}

func TestSignalImageRepositoryGetBySignalID(t *testing.T) {
	exp := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	pool := &imageRepoStubPool{
		queryRowValues: []any{int64(9), "image/png", int32(100), int32(80), exp, []byte{0x89, 0x50, 0x4e, 0x47}},
	}
	repo := NewSignalImageRepository(pool, trace.NewNoopTracerProvider().Tracer("test"))

	got, err := repo.GetSignalImageBySignalID(context.Background(), 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Ref.ImageID != 9 || len(got.Bytes) == 0 {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestSignalImageRepositoryListRetryCandidates(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pool := &imageRepoStubPool{
		rowsData: [][]any{{
			int64(31), "BTC", "1h", domain.IndicatorRSI, string(domain.DirectionLong), int16(domain.RiskLevel2), now, "retry me",
		}},
	}
	repo := NewSignalImageRepository(pool, trace.NewNoopTracerProvider().Tracer("test"))

	list, err := repo.ListRetryCandidates(context.Background(), 10, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 || list[0].ID != 31 || list[0].Symbol != "BTC" {
		t.Fatalf("unexpected retry candidates: %+v", list)
	}
}

func TestSignalImageRepositoryDeleteExpired(t *testing.T) {
	pool := &imageRepoStubPool{execRowsAffected: 3}
	repo := NewSignalImageRepository(pool, trace.NewNoopTracerProvider().Tracer("test"))

	deleted, err := repo.DeleteExpiredSignalImages(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 deleted rows, got %d", deleted)
	}
}

type imageRepoStubPool struct {
	execRowsAffected int64
	rowsData         [][]any
	queryRowValues   []any
	queryRowErr      error
}

func (s *imageRepoStubPool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag(fmt.Sprintf("DELETE %d", s.execRowsAffected)), nil
}

func (s *imageRepoStubPool) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return &signalStubBatchResults{}
}

func (s *imageRepoStubPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if s.rowsData == nil {
		return &signalStubRows{}, nil
	}
	dataCopy := make([][]any, len(s.rowsData))
	for i := range s.rowsData {
		row := make([]any, len(s.rowsData[i]))
		copy(row, s.rowsData[i])
		dataCopy[i] = row
	}
	return &signalStubRows{data: dataCopy}, nil
}

func (s *imageRepoStubPool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return &imageRepoStubRow{values: s.queryRowValues, err: s.queryRowErr}
}

type imageRepoStubRow struct {
	values []any
	err    error
}

func (r *imageRepoStubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("destination count mismatch")
	}
	for i, d := range dest {
		switch ptr := d.(type) {
		case *int64:
			*ptr = r.values[i].(int64)
		case *string:
			*ptr = r.values[i].(string)
		case *int:
			switch v := r.values[i].(type) {
			case int:
				*ptr = v
			case int32:
				*ptr = int(v)
			default:
				return fmt.Errorf("unexpected int type %T", r.values[i])
			}
		case *time.Time:
			*ptr = r.values[i].(time.Time)
		case *[]byte:
			*ptr = append([]byte(nil), r.values[i].([]byte)...)
		default:
			return fmt.Errorf("unsupported scan type %T", d)
		}
	}
	return nil
}
