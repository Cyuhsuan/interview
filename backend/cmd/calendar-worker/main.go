// Command calendar-worker is the outbox delivery worker described in
// backend/README.md's directory layout and "PostgreSQL-first 預約一致性"
// steps 5-6: it polls appointment_outbox for due rows and hands them to
// internal/service/calendar, which never changes appointment status.
package main

import (
	"context"
	"log"
	"time"

	"go.uber.org/fx"
	"gorm.io/gorm"

	calendarAdapter "backend/internal/calendar"
	"backend/internal/model"
	"backend/internal/platform/config"
	"backend/internal/platform/database"
	calendarRepo "backend/internal/repository/calendar"
	calendarService "backend/internal/service/calendar"
)

func newCalendarAdapters() map[string]calendarService.Adapter {
	sandbox := calendarAdapter.NewSandboxAdapter()
	return map[string]calendarService.Adapter{
		model.CalendarProviderGoogle:    sandbox,
		model.CalendarProviderMicrosoft: sandbox,
	}
}

func newCalendarService(
	repo calendarService.OutboxRepository,
	adapters map[string]calendarService.Adapter,
	cfg config.Config,
) *calendarService.Service {
	return calendarService.NewService(
		repo, adapters,
		cfg.CalendarOutboxMaxAttempts,
		time.Duration(cfg.CalendarOutboxRetryBackoffSeconds)*time.Second,
	)
}

// runWorker starts the polling loop as an Fx lifecycle hook: OnStart
// launches the goroutine, OnStop cancels its context and waits for it to
// return, so a shutdown never leaves an outbox row stuck in "processing".
func runWorker(lc fx.Lifecycle, svc *calendarService.Service, cfg config.Config) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	pollInterval := time.Duration(cfg.CalendarWorkerPollIntervalSeconds) * time.Second

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(done)
				ticker := time.NewTicker(pollInterval)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						drainDue(ctx, svc)
					}
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			<-done
			return nil
		},
	})
}

// drainDue processes every currently-due outbox row before waiting for the
// next tick, instead of at most one per poll interval.
func drainDue(ctx context.Context, svc *calendarService.Service) {
	for {
		processed, err := svc.ProcessOne(ctx)
		if err != nil {
			log.Printf("calendar-worker: outbox delivery attempt failed: %v", err)
			continue
		}
		if !processed {
			return
		}
	}
}

func main() {
	fx.New(
		fx.Provide(
			config.Load,
			database.New,

			calendarRepo.NewRepository,
			func(r *calendarRepo.Repository) calendarService.OutboxRepository { return r },
			newCalendarAdapters,
			newCalendarService,
		),
		fx.Invoke(
			func(*gorm.DB) {}, // force the pool to be constructed and pinged at startup
			runWorker,
		),
	).Run()
}
