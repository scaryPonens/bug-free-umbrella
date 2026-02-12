package job

import (
	"context"
	"log"
	"time"

	"go.opentelemetry.io/otel/trace"
)

const (
	defaultImageRetryBatchSize = 20
	imageRetryTick             = 5 * time.Minute
	imageCleanupTick           = time.Hour
)

type SignalImageMaintainer interface {
	RetryFailedImages(ctx context.Context, limit int) (int, error)
	DeleteExpiredSignalImages(ctx context.Context) (int64, error)
}

type SignalImageMaintenance struct {
	tracer   trace.Tracer
	maintain SignalImageMaintainer
}

func NewSignalImageMaintenance(tracer trace.Tracer, maintain SignalImageMaintainer) *SignalImageMaintenance {
	return &SignalImageMaintenance{
		tracer:   tracer,
		maintain: maintain,
	}
}

func (j *SignalImageMaintenance) Start(ctx context.Context) {
	if j == nil || j.maintain == nil {
		<-ctx.Done()
		return
	}

	log.Println("Signal image maintenance starting...")
	retryTicker := time.NewTicker(imageRetryTick)
	cleanupTicker := time.NewTicker(imageCleanupTick)
	defer retryTicker.Stop()
	defer cleanupTicker.Stop()

	j.runRetry(ctx)
	j.runCleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("Signal image maintenance stopped")
			return
		case <-retryTicker.C:
			j.runRetry(ctx)
		case <-cleanupTicker.C:
			j.runCleanup(ctx)
		}
	}
}

func (j *SignalImageMaintenance) runRetry(ctx context.Context) {
	if j.tracer != nil {
		_, span := j.tracer.Start(ctx, "signal-image-job.retry")
		defer span.End()
	}
	count, err := j.maintain.RetryFailedImages(ctx, defaultImageRetryBatchSize)
	if err != nil {
		log.Printf("signal image retry error: %v", err)
		return
	}
	if count > 0 {
		log.Printf("signal image retry succeeded for %d signal(s)", count)
	}
}

func (j *SignalImageMaintenance) runCleanup(ctx context.Context) {
	if j.tracer != nil {
		_, span := j.tracer.Start(ctx, "signal-image-job.cleanup")
		defer span.End()
	}
	deleted, err := j.maintain.DeleteExpiredSignalImages(ctx)
	if err != nil {
		log.Printf("signal image cleanup error: %v", err)
		return
	}
	if deleted > 0 {
		log.Printf("signal image cleanup removed %d row(s)", deleted)
	}
}
