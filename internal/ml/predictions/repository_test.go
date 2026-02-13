package predictions

import (
	"context"
	"fmt"
	"testing"
	"time"

	"bug-free-umbrella/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel/trace"
)

func TestUpsertPredictionIdempotentForAnomaly(t *testing.T) {
	pool := newPredictionPoolStub()
	repo := NewRepository(pool, trace.NewNoopTracerProvider().Tracer("predictions-test"))

	openTime := time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)
	targetTime := openTime.Add(4 * time.Hour)
	prediction := domain.MLPrediction{
		Symbol:       "BTC",
		Interval:     "4h",
		OpenTime:     openTime,
		TargetTime:   targetTime,
		ModelKey:     "iforest_4h",
		ModelVersion: 1,
		ProbUp:       0.5,
		Confidence:   0.82,
		Direction:    domain.DirectionHold,
		Risk:         domain.RiskLevel3,
		DetailsJSON:  `{"anomaly_score":0.82}`,
	}

	first, err := repo.UpsertPrediction(context.Background(), prediction)
	if err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	prediction.Confidence = 0.91
	prediction.DetailsJSON = "invalid-json"
	second, err := repo.UpsertPrediction(context.Background(), prediction)
	if err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected idempotent upsert to keep same id, got first=%d second=%d", first.ID, second.ID)
	}
	if second.Confidence != 0.91 {
		t.Fatalf("expected updated confidence, got %.4f", second.Confidence)
	}
	if second.DetailsJSON != `{"raw":"invalid"}` {
		t.Fatalf("expected invalid details to be normalized, got %s", second.DetailsJSON)
	}
}

func TestAttachSignalID(t *testing.T) {
	pool := newPredictionPoolStub()
	repo := NewRepository(pool, trace.NewNoopTracerProvider().Tracer("predictions-test"))

	openTime := time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)
	targetTime := openTime.Add(4 * time.Hour)
	prediction, err := repo.UpsertPrediction(context.Background(), domain.MLPrediction{
		Symbol:       "BTC",
		Interval:     "1h",
		OpenTime:     openTime,
		TargetTime:   targetTime,
		ModelKey:     "logreg",
		ModelVersion: 1,
		ProbUp:       0.9,
		Confidence:   0.8,
		Direction:    domain.DirectionLong,
		Risk:         domain.RiskLevel2,
		DetailsJSON:  "{}",
	})
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if err := repo.AttachSignalID(context.Background(), prediction.ID, 999); err != nil {
		t.Fatalf("attach signal id failed: %v", err)
	}
}

type predictionPoolStub struct {
	nextID int64
	rows   map[string]predictionRecord
}

type predictionRecord struct {
	id             int64
	symbol         string
	interval       string
	openTime       time.Time
	targetTime     time.Time
	modelKey       string
	modelVersion   int
	probUp         float64
	confidence     float64
	direction      string
	risk           int16
	signalID       *int64
	detailsJSON    string
	createdAt      time.Time
	resolvedAt     *time.Time
	actualUp       *bool
	isCorrect      *bool
	realizedReturn *float64
}

func newPredictionPoolStub() *predictionPoolStub {
	return &predictionPoolStub{
		nextID: 1,
		rows:   make(map[string]predictionRecord),
	}
}

func (s *predictionPoolStub) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if len(args) >= 2 && len(sql) > 0 {
		predID, ok := args[0].(int64)
		if ok {
			for key, row := range s.rows {
				if row.id == predID {
					sid := args[1].(int64)
					row.signalID = &sid
					s.rows[key] = row
					return pgconn.NewCommandTag("UPDATE 1"), nil
				}
			}
		}
	}
	return pgconn.NewCommandTag("UPDATE 0"), nil
}

func (s *predictionPoolStub) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return &predictionRowsStub{}, nil
}

func (s *predictionPoolStub) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	key := fmt.Sprintf("%s|%s|%d|%s|%d", args[0], args[1], args[2].(time.Time).Unix(), args[4], args[5])
	record, ok := s.rows[key]
	if !ok {
		record = predictionRecord{
			id:        s.nextID,
			createdAt: time.Date(2026, 2, 13, 0, 0, 0, 0, time.UTC),
		}
		s.nextID++
	}
	record.symbol = args[0].(string)
	record.interval = args[1].(string)
	record.openTime = args[2].(time.Time)
	record.targetTime = args[3].(time.Time)
	record.modelKey = args[4].(string)
	record.modelVersion = args[5].(int)
	record.probUp = args[6].(float64)
	record.confidence = args[7].(float64)
	record.direction = args[8].(string)
	record.risk = args[9].(int16)
	if signalPtr, ok := args[10].(*int64); ok && signalPtr != nil {
		v := *signalPtr
		record.signalID = &v
	} else if args[10] == nil {
		record.signalID = nil
	}
	record.detailsJSON = args[11].(string)
	s.rows[key] = record

	return predictionRowStub{record: record}
}

type predictionRowStub struct {
	record predictionRecord
}

func (r predictionRowStub) Scan(dest ...any) error {
	values := []any{
		r.record.id,
		r.record.symbol,
		r.record.interval,
		r.record.openTime,
		r.record.targetTime,
		r.record.modelKey,
		r.record.modelVersion,
		r.record.probUp,
		r.record.confidence,
		r.record.direction,
		r.record.risk,
		r.record.signalID,
		r.record.detailsJSON,
		r.record.createdAt,
		r.record.resolvedAt,
		r.record.actualUp,
		r.record.isCorrect,
		r.record.realizedReturn,
	}
	for i, d := range dest {
		switch ptr := d.(type) {
		case *int64:
			*ptr = values[i].(int64)
		case *string:
			*ptr = values[i].(string)
		case *time.Time:
			*ptr = values[i].(time.Time)
		case *int:
			*ptr = values[i].(int)
		case *float64:
			*ptr = values[i].(float64)
		case *int16:
			*ptr = values[i].(int16)
		case **int64:
			v, ok := values[i].(*int64)
			if !ok || v == nil {
				*ptr = nil
			} else {
				copyV := *v
				*ptr = &copyV
			}
		case *pgtype.Timestamptz:
			v, ok := values[i].(*time.Time)
			if !ok || v == nil {
				*ptr = pgtype.Timestamptz{}
			} else {
				*ptr = pgtype.Timestamptz{Time: *v, Valid: true}
			}
		case *pgtype.Bool:
			v, ok := values[i].(*bool)
			if !ok || v == nil {
				*ptr = pgtype.Bool{}
			} else {
				*ptr = pgtype.Bool{Bool: *v, Valid: true}
			}
		case *pgtype.Float8:
			v, ok := values[i].(*float64)
			if !ok || v == nil {
				*ptr = pgtype.Float8{}
			} else {
				*ptr = pgtype.Float8{Float64: *v, Valid: true}
			}
		default:
			return fmt.Errorf("unsupported scan type %T", d)
		}
	}
	return nil
}

type predictionRowsStub struct{}

func (r *predictionRowsStub) Close()                                       {}
func (r *predictionRowsStub) Err() error                                   { return nil }
func (r *predictionRowsStub) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *predictionRowsStub) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *predictionRowsStub) Next() bool                                   { return false }
func (r *predictionRowsStub) Scan(...any) error                            { return nil }
func (r *predictionRowsStub) Values() ([]any, error)                       { return nil, nil }
func (r *predictionRowsStub) RawValues() [][]byte                          { return nil }
func (r *predictionRowsStub) Conn() *pgx.Conn                              { return nil }
