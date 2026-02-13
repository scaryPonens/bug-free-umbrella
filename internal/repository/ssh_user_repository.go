package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/trace"
)

type SSHUser struct {
	ID          int64
	Username    string
	DisplayName string
	PublicKey   string
	KeyType     string
	Fingerprint string
	IsActive    bool
	LastLoginAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type SSHUserRepository struct {
	pool   PgxPool
	tracer trace.Tracer
}

func NewSSHUserRepository(pool PgxPool, tracer trace.Tracer) *SSHUserRepository {
	return &SSHUserRepository{pool: pool, tracer: tracer}
}

func (r *SSHUserRepository) FindByFingerprint(ctx context.Context, fingerprint string) (*SSHUser, error) {
	_, span := r.tracer.Start(ctx, "ssh-user-repo.find-by-fingerprint")
	defer span.End()

	row := r.pool.QueryRow(ctx,
		`SELECT id, username, display_name, public_key, key_type, fingerprint,
		        is_active, last_login_at, created_at, updated_at
		 FROM ssh_users
		 WHERE fingerprint = $1 AND is_active = TRUE`,
		fingerprint,
	)

	var u SSHUser
	var lastLogin *time.Time
	err := row.Scan(
		&u.ID, &u.Username, &u.DisplayName, &u.PublicKey, &u.KeyType,
		&u.Fingerprint, &u.IsActive, &lastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.LastLoginAt = lastLogin
	return &u, nil
}

func (r *SSHUserRepository) UpdateLastLogin(ctx context.Context, userID int64) error {
	_, span := r.tracer.Start(ctx, "ssh-user-repo.update-last-login")
	defer span.End()

	_, err := r.pool.Exec(ctx,
		`UPDATE ssh_users SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

func (r *SSHUserRepository) ListActive(ctx context.Context) ([]SSHUser, error) {
	_, span := r.tracer.Start(ctx, "ssh-user-repo.list-active")
	defer span.End()

	rows, err := r.pool.Query(ctx,
		`SELECT id, username, display_name, public_key, key_type, fingerprint,
		        is_active, last_login_at, created_at, updated_at
		 FROM ssh_users
		 WHERE is_active = TRUE
		 ORDER BY username ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []SSHUser
	for rows.Next() {
		var u SSHUser
		var lastLogin *time.Time
		if err := rows.Scan(
			&u.ID, &u.Username, &u.DisplayName, &u.PublicKey, &u.KeyType,
			&u.Fingerprint, &u.IsActive, &lastLogin, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		u.LastLoginAt = lastLogin
		users = append(users, u)
	}
	return users, rows.Err()
}
