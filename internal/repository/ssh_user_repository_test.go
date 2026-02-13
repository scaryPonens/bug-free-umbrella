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

func TestSSHUserFindByFingerprintReturnsUser(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pool := &sshStubPool{
		queryRowData: []any{
			int64(1), "alice", "Alice", "ssh-ed25519 AAAA...", "ssh-ed25519",
			"SHA256:abc123", true, (*time.Time)(nil), now, now,
		},
	}
	repo := NewSSHUserRepository(pool, trace.NewNoopTracerProvider().Tracer("test"))

	user, err := repo.FindByFingerprint(context.Background(), "SHA256:abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.ID != 1 {
		t.Fatalf("expected ID 1, got %d", user.ID)
	}
	if user.Username != "alice" {
		t.Fatalf("expected username alice, got %s", user.Username)
	}
	if user.Fingerprint != "SHA256:abc123" {
		t.Fatalf("expected fingerprint SHA256:abc123, got %s", user.Fingerprint)
	}
}

func TestSSHUserFindByFingerprintNotFound(t *testing.T) {
	pool := &sshStubPool{queryRowErr: pgx.ErrNoRows}
	repo := NewSSHUserRepository(pool, trace.NewNoopTracerProvider().Tracer("test"))

	user, err := repo.FindByFingerprint(context.Background(), "SHA256:unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user != nil {
		t.Fatalf("expected nil user, got %+v", user)
	}
}

func TestSSHUserUpdateLastLoginExecs(t *testing.T) {
	pool := &sshStubPool{}
	repo := NewSSHUserRepository(pool, trace.NewNoopTracerProvider().Tracer("test"))

	if err := repo.UpdateLastLogin(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool.execCount != 1 {
		t.Fatalf("expected 1 exec, got %d", pool.execCount)
	}
}

func TestSSHUserListActiveReturnsUsers(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pool := &sshStubPool{
		rowsData: [][]any{
			{int64(1), "alice", "Alice", "ssh-ed25519 AAAA...", "ssh-ed25519", "SHA256:abc", true, (*time.Time)(nil), now, now},
			{int64(2), "bob", "Bob", "ssh-ed25519 BBBB...", "ssh-ed25519", "SHA256:def", true, (*time.Time)(nil), now, now},
		},
	}
	repo := NewSSHUserRepository(pool, trace.NewNoopTracerProvider().Tracer("test"))

	users, err := repo.ListActive(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].Username != "alice" {
		t.Fatalf("expected alice, got %s", users[0].Username)
	}
}

// --- stubs ---

type sshStubPool struct {
	execCount    int
	queryRowData []any
	queryRowErr  error
	rowsData     [][]any
}

func (s *sshStubPool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	s.execCount++
	return pgconn.CommandTag{}, nil
}

func (s *sshStubPool) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return &sshStubBatchResults{}
}

func (s *sshStubPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if s.rowsData == nil {
		return &sshStubRows{}, nil
	}
	dataCopy := make([][]any, len(s.rowsData))
	for i := range s.rowsData {
		row := make([]any, len(s.rowsData[i]))
		copy(row, s.rowsData[i])
		dataCopy[i] = row
	}
	return &sshStubRows{data: dataCopy}, nil
}

func (s *sshStubPool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return &sshStubRow{data: s.queryRowData, err: s.queryRowErr}
}

type sshStubBatchResults struct{}

func (sshStubBatchResults) Exec() (pgconn.CommandTag, error) { return pgconn.CommandTag{}, nil }
func (sshStubBatchResults) Query() (pgx.Rows, error)         { return &sshStubRows{}, nil }
func (sshStubBatchResults) QueryRow() pgx.Row                { return &sshStubRow{} }
func (sshStubBatchResults) Close() error                     { return nil }

type sshStubRows struct {
	data [][]any
	idx  int
}

func (r *sshStubRows) Close()                                       {}
func (r *sshStubRows) Err() error                                   { return nil }
func (r *sshStubRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *sshStubRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *sshStubRows) Next() bool {
	if len(r.data) == 0 || r.idx >= len(r.data) {
		return false
	}
	r.idx++
	return true
}

func (r *sshStubRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.data) {
		return fmt.Errorf("invalid scan index")
	}
	row := r.data[r.idx-1]
	for i, d := range dest {
		switch ptr := d.(type) {
		case *int64:
			*ptr = row[i].(int64)
		case *string:
			*ptr = row[i].(string)
		case *bool:
			*ptr = row[i].(bool)
		case **time.Time:
			if row[i] == nil || row[i] == (*time.Time)(nil) {
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

func (r *sshStubRows) Values() ([]any, error) { return nil, nil }
func (r *sshStubRows) RawValues() [][]byte    { return nil }
func (r *sshStubRows) Conn() *pgx.Conn        { return nil }

type sshStubRow struct {
	data []any
	err  error
}

func (r *sshStubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.data == nil {
		return pgx.ErrNoRows
	}
	for i, d := range dest {
		switch ptr := d.(type) {
		case *int64:
			*ptr = r.data[i].(int64)
		case *string:
			*ptr = r.data[i].(string)
		case *bool:
			*ptr = r.data[i].(bool)
		case **time.Time:
			if r.data[i] == nil || r.data[i] == (*time.Time)(nil) {
				*ptr = nil
			} else {
				v := r.data[i].(time.Time)
				*ptr = &v
			}
		case *time.Time:
			*ptr = r.data[i].(time.Time)
		default:
			return fmt.Errorf("unsupported dest type %T", d)
		}
	}
	return nil
}
