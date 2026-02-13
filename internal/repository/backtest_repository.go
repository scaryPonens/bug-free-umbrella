package repository

import (
	"context"
	"time"

	"bug-free-umbrella/internal/domain"

	"go.opentelemetry.io/otel/trace"
)

type DailyAccuracy struct {
	ModelKey string
	DayUTC   time.Time
	Total    int
	Correct  int
	Accuracy float64
}

type BacktestRepository struct {
	pool   PgxPool
	tracer trace.Tracer
}

func NewBacktestRepository(pool PgxPool, tracer trace.Tracer) *BacktestRepository {
	return &BacktestRepository{pool: pool, tracer: tracer}
}

func (r *BacktestRepository) GetDailyAccuracy(ctx context.Context, modelKey string, days int) ([]DailyAccuracy, error) {
	_, span := r.tracer.Start(ctx, "backtest-repo.get-daily-accuracy")
	defer span.End()

	if days <= 0 {
		days = 30
	}

	rows, err := r.pool.Query(ctx,
		`SELECT model_key, day_utc, total, correct, accuracy
		 FROM ml_accuracy_daily
		 WHERE model_key = $1
		 ORDER BY day_utc DESC
		 LIMIT $2`,
		modelKey, days,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DailyAccuracy
	for rows.Next() {
		var d DailyAccuracy
		if err := rows.Scan(&d.ModelKey, &d.DayUTC, &d.Total, &d.Correct, &d.Accuracy); err != nil {
			return nil, err
		}
		d.DayUTC = d.DayUTC.UTC()
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *BacktestRepository) GetAccuracySummary(ctx context.Context) ([]DailyAccuracy, error) {
	_, span := r.tracer.Start(ctx, "backtest-repo.get-accuracy-summary")
	defer span.End()

	rows, err := r.pool.Query(ctx,
		`SELECT model_key,
		        NOW() AS day_utc,
		        SUM(total)::INT AS total,
		        SUM(correct)::INT AS correct,
		        CASE WHEN SUM(total) = 0 THEN 0
		             ELSE SUM(correct)::DOUBLE PRECISION / SUM(total)::DOUBLE PRECISION
		        END AS accuracy
		 FROM ml_accuracy_daily
		 GROUP BY model_key
		 ORDER BY model_key`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DailyAccuracy
	for rows.Next() {
		var d DailyAccuracy
		if err := rows.Scan(&d.ModelKey, &d.DayUTC, &d.Total, &d.Correct, &d.Accuracy); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *BacktestRepository) ListRecentPredictions(ctx context.Context, limit int) ([]domain.MLPrediction, error) {
	_, span := r.tracer.Start(ctx, "backtest-repo.list-recent-predictions")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, symbol, interval, open_time, target_time,
		        model_key, model_version, prob_up, confidence,
		        direction, risk, signal_id, details_json, created_at,
		        resolved_at, actual_up, is_correct, realized_return
		 FROM ml_predictions
		 WHERE resolved_at IS NOT NULL
		 ORDER BY resolved_at DESC
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.MLPrediction
	for rows.Next() {
		var p domain.MLPrediction
		var direction string
		var risk int16
		if err := rows.Scan(
			&p.ID, &p.Symbol, &p.Interval, &p.OpenTime, &p.TargetTime,
			&p.ModelKey, &p.ModelVersion, &p.ProbUp, &p.Confidence,
			&direction, &risk, &p.SignalID, &p.DetailsJSON, &p.CreatedAt,
			&p.ResolvedAt, &p.ActualUp, &p.IsCorrect, &p.RealizedReturn,
		); err != nil {
			return nil, err
		}
		p.Direction = domain.SignalDirection(direction)
		p.Risk = domain.RiskLevel(risk)
		out = append(out, p)
	}
	return out, rows.Err()
}
