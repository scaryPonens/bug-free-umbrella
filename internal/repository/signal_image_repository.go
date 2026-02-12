package repository

import (
	"context"
	"time"

	"bug-free-umbrella/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/trace"
)

type SignalImageRepository struct {
	pool   PgxPool
	tracer trace.Tracer
}

func NewSignalImageRepository(pool PgxPool, tracer trace.Tracer) *SignalImageRepository {
	return &SignalImageRepository{pool: pool, tracer: tracer}
}

func (r *SignalImageRepository) UpsertSignalImageReady(
	ctx context.Context,
	signalID int64,
	imageBytes []byte,
	mimeType string,
	width, height int,
	expiresAt time.Time,
) (*domain.SignalImageRef, error) {
	_, span := r.tracer.Start(ctx, "signal-image-repo.upsert-ready")
	defer span.End()

	var out domain.SignalImageRef
	err := r.pool.QueryRow(ctx, `
INSERT INTO signal_images (
    signal_id, mime_type, image_bytes, width, height, render_status, error_text, retry_count, next_retry_at, expires_at
) VALUES ($1, $2, $3, $4, $5, 'ready', '', 0, NOW(), $6)
ON CONFLICT (signal_id) DO UPDATE SET
    mime_type = EXCLUDED.mime_type,
    image_bytes = EXCLUDED.image_bytes,
    width = EXCLUDED.width,
    height = EXCLUDED.height,
    render_status = 'ready',
    error_text = '',
    retry_count = 0,
    next_retry_at = NOW(),
    expires_at = EXCLUDED.expires_at
RETURNING id, mime_type, width, height, expires_at
`, signalID, mimeType, imageBytes, width, height, expiresAt.UTC()).
		Scan(&out.ImageID, &out.MimeType, &out.Width, &out.Height, &out.ExpiresAt)
	if err != nil {
		return nil, err
	}
	out.ExpiresAt = out.ExpiresAt.UTC()
	return &out, nil
}

func (r *SignalImageRepository) UpsertSignalImageFailure(
	ctx context.Context,
	signalID int64,
	errorText string,
	nextRetryAt time.Time,
	expiresAt time.Time,
) error {
	_, span := r.tracer.Start(ctx, "signal-image-repo.upsert-failure")
	defer span.End()

	_, err := r.pool.Exec(ctx, `
INSERT INTO signal_images (
    signal_id, mime_type, image_bytes, width, height, render_status, error_text, retry_count, next_retry_at, expires_at
) VALUES ($1, 'image/png', ''::bytea, 0, 0, 'failed', $2, 1, $3, $4)
ON CONFLICT (signal_id) DO UPDATE SET
    render_status = 'failed',
    error_text = EXCLUDED.error_text,
    retry_count = signal_images.retry_count + 1,
    next_retry_at = EXCLUDED.next_retry_at,
    expires_at = EXCLUDED.expires_at
`, signalID, errorText, nextRetryAt.UTC(), expiresAt.UTC())
	return err
}

func (r *SignalImageRepository) GetSignalImageBySignalID(
	ctx context.Context,
	signalID int64,
) (*domain.SignalImageData, error) {
	_, span := r.tracer.Start(ctx, "signal-image-repo.get-by-signal-id")
	defer span.End()

	var out domain.SignalImageData
	err := r.pool.QueryRow(ctx, `
SELECT id, mime_type, width, height, expires_at, image_bytes
FROM signal_images
WHERE signal_id = $1
  AND render_status = 'ready'
  AND expires_at > NOW()
`, signalID).Scan(
		&out.Ref.ImageID,
		&out.Ref.MimeType,
		&out.Ref.Width,
		&out.Ref.Height,
		&out.Ref.ExpiresAt,
		&out.Bytes,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	out.Ref.ExpiresAt = out.Ref.ExpiresAt.UTC()
	return &out, nil
}

func (r *SignalImageRepository) ListRetryCandidates(
	ctx context.Context,
	limit int,
	maxRetryCount int,
) ([]domain.Signal, error) {
	_, span := r.tracer.Start(ctx, "signal-image-repo.list-retry-candidates")
	defer span.End()

	if limit <= 0 {
		limit = 20
	}
	if maxRetryCount <= 0 {
		maxRetryCount = 3
	}

	rows, err := r.pool.Query(ctx, `
SELECT s.id, s.symbol, s.interval, s.indicator, s.direction, s.risk, s.timestamp, s.details
FROM signal_images si
JOIN signals s ON s.id = si.signal_id
WHERE si.render_status = 'failed'
  AND si.retry_count < $1
  AND si.next_retry_at <= NOW()
ORDER BY si.next_retry_at ASC
LIMIT $2
`, maxRetryCount, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Signal, 0, limit)
	for rows.Next() {
		var s domain.Signal
		var direction string
		var risk int16
		if err := rows.Scan(
			&s.ID,
			&s.Symbol,
			&s.Interval,
			&s.Indicator,
			&direction,
			&risk,
			&s.Timestamp,
			&s.Details,
		); err != nil {
			return nil, err
		}
		s.Direction = domain.SignalDirection(direction)
		s.Risk = domain.RiskLevel(risk)
		s.Timestamp = s.Timestamp.UTC()
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SignalImageRepository) DeleteExpiredSignalImages(ctx context.Context) (int64, error) {
	_, span := r.tracer.Start(ctx, "signal-image-repo.delete-expired")
	defer span.End()

	tag, err := r.pool.Exec(ctx, `DELETE FROM signal_images WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
