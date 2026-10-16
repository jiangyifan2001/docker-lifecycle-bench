package runner

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"docker-lifecycle-bench/internal/campaign"
	"docker-lifecycle-bench/internal/model"
)

type Executor interface {
	Cycle(context.Context, model.Request) model.Operation
}

type Writer interface {
	WriteOperation(model.Operation) error
}

type Runner struct {
	Spec     campaign.Spec
	RunID    string
	Executor Executor
	Writer   Writer
	nextID   atomic.Uint64

	mu         sync.Mutex
	operations []model.Operation
}

func (r *Runner) Run(ctx context.Context) ([]model.Operation, error) {
	if err := r.runWarmup(ctx); err != nil {
		return r.snapshot(), err
	}
	if err := r.runLatency(ctx); err != nil {
		return r.snapshot(), err
	}
	if r.Spec.Capacity.Enabled {
		for _, workers := range r.Spec.Capacity.Workers {
			if err := r.runCapacityPoint(ctx, workers); err != nil {
				return r.snapshot(), err
			}
		}
	}
	return r.snapshot(), nil
}

func (r *Runner) runWarmup(ctx context.Context) error {
	started := time.Now()
	attempts := 0
	errorsSeen := 0
	for attempts < r.Spec.Warmup.MinCycles || time.Since(started) < r.Spec.Warmup.MinDuration.Duration {
		operation, err := r.execute(ctx, model.PhaseWarmup, 1, 0)
		attempts++
		if err != nil {
			return err
		}
		if operation.Status != model.StatusSuccess {
			errorsSeen++
			if errorsSeen >= 3 {
				return fmt.Errorf("warmup aborted after %d failed cycles", errorsSeen)
			}
		}
	}
	if errorsSeen > 0 {
		return fmt.Errorf("warmup completed with %d failed cycles", errorsSeen)
	}
	return nil
}

func (r *Runner) runLatency(ctx context.Context) error {
	errorsSeen := 0
	for range r.Spec.Measurement.Samples {
		operation, err := r.execute(ctx, model.PhaseLatency, 1, 0)
		if err != nil {
			return err
		}
		if operation.Status != model.StatusSuccess {
			errorsSeen++
			if errorsSeen >= 3 {
				return fmt.Errorf("latency run aborted after %d failed cycles", errorsSeen)
			}
		}
	}
	if errorsSeen > 0 {
		return fmt.Errorf("latency run completed with %d failed cycles", errorsSeen)
	}
	return nil
}

func (r *Runner) runCapacityPoint(ctx context.Context, workers int) error {
	deadline := time.Now().Add(r.Spec.Capacity.DurationPerPoint.Duration)
	var wait sync.WaitGroup
	var failures atomic.Int64
	var writerError error
	var writerErrorMu sync.Mutex

	for worker := range workers {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			consecutiveFailures := 0
			for time.Now().Before(deadline) && ctx.Err() == nil {
				operation, err := r.execute(ctx, model.PhaseCapacity, workers, worker)
				if err != nil {
					writerErrorMu.Lock()
					if writerError == nil {
						writerError = err
					}
					writerErrorMu.Unlock()
					return
				}
				if operation.Status == model.StatusSuccess {
					consecutiveFailures = 0
					continue
				}
				failures.Add(1)
				consecutiveFailures++
				if consecutiveFailures >= 3 {
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
		}(worker)
	}
	wait.Wait()
	writerErrorMu.Lock()
	defer writerErrorMu.Unlock()
	if writerError != nil {
		return writerError
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if failures.Load() > 0 {
		return fmt.Errorf("capacity point with %d workers had %d failed cycles", workers, failures.Load())
	}
	return nil
}

func (r *Runner) execute(ctx context.Context, phase string, concurrency, worker int) (model.Operation, error) {
	cycleContext, cancel := context.WithTimeout(ctx, r.Spec.Measurement.CycleTimeout.Duration)
	defer cancel()
	request := model.Request{
		RunID:       r.RunID,
		SampleID:    r.nextID.Add(1),
		Phase:       phase,
		Concurrency: concurrency,
		Worker:      worker,
	}
	operation := r.Executor.Cycle(cycleContext, request)
	if err := r.Writer.WriteOperation(operation); err != nil {
		return operation, fmt.Errorf("write operation: %w", err)
	}
	r.mu.Lock()
	r.operations = append(r.operations, operation)
	r.mu.Unlock()
	return operation, nil
}

func (r *Runner) snapshot() []model.Operation {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]model.Operation, len(r.operations))
	copy(result, r.operations)
	return result
}
