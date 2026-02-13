package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace"
)

func TestNextVersionWithIntervalNamespacedKey(t *testing.T) {
	pool := &registryPoolStub{
		queryRowFunc: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return registryRowStub{values: []any{7}}
		},
	}
	repo := NewRepository(pool, trace.NewNoopTracerProvider().Tracer("registry-test"))

	version, err := repo.NextVersion(context.Background(), "iforest_4h")
	if err != nil {
		t.Fatalf("next version failed: %v", err)
	}
	if version != 7 {
		t.Fatalf("expected version 7, got %d", version)
	}
}

func TestActivateModel(t *testing.T) {
	pool := &registryPoolStub{}
	tx := &registryTxStub{
		execResults: []pgconn.CommandTag{
			pgconn.NewCommandTag("UPDATE 2"),
			pgconn.NewCommandTag("UPDATE 1"),
		},
	}
	pool.beginTx = tx
	repo := NewRepository(pool, trace.NewNoopTracerProvider().Tracer("registry-test"))

	if err := repo.ActivateModel(context.Background(), "iforest_1h", 2); err != nil {
		t.Fatalf("activate failed: %v", err)
	}
	if !tx.committed {
		t.Fatal("expected transaction commit")
	}
}

func TestActivateModelNoRows(t *testing.T) {
	pool := &registryPoolStub{}
	tx := &registryTxStub{
		execResults: []pgconn.CommandTag{
			pgconn.NewCommandTag("UPDATE 2"),
			pgconn.NewCommandTag("UPDATE 0"),
		},
	}
	pool.beginTx = tx
	repo := NewRepository(pool, trace.NewNoopTracerProvider().Tracer("registry-test"))

	err := repo.ActivateModel(context.Background(), "iforest_1h", 2)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}

type registryPoolStub struct {
	beginTx      pgx.Tx
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (s *registryPoolStub) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (s *registryPoolStub) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if s.queryRowFunc != nil {
		return s.queryRowFunc(ctx, sql, args...)
	}
	return registryRowStub{}
}

func (s *registryPoolStub) Begin(_ context.Context) (pgx.Tx, error) {
	return s.beginTx, nil
}

type registryTxStub struct {
	execResults []pgconn.CommandTag
	execCalls   int
	committed   bool
}

func (s *registryTxStub) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	if s.execCalls >= len(s.execResults) {
		return pgconn.NewCommandTag("UPDATE 0"), nil
	}
	tag := s.execResults[s.execCalls]
	s.execCalls++
	return tag, nil
}

func (s *registryTxStub) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return registryRowStub{}
}

func (s *registryTxStub) Commit(_ context.Context) error {
	s.committed = true
	return nil
}

func (s *registryTxStub) Rollback(_ context.Context) error { return nil }

func (s *registryTxStub) Begin(context.Context) (pgx.Tx, error) { return nil, nil }
func (s *registryTxStub) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (s *registryTxStub) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (s *registryTxStub) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (s *registryTxStub) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (s *registryTxStub) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }
func (s *registryTxStub) Conn() *pgx.Conn                                         { return nil }

type registryRowStub struct {
	values []any
	err    error
}

func (r registryRowStub) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i := range dest {
		switch d := dest[i].(type) {
		case *int:
			if len(r.values) > i {
				*d = r.values[i].(int)
			}
		}
	}
	return nil
}
