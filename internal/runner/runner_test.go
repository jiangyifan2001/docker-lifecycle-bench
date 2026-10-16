package runner

import (
	"context"
	"sync"
	"testing"
	"time"

	"docker-lifecycle-bench/internal/campaign"
	"docker-lifecycle-bench/internal/model"
)

type fakeExecutor struct{}

func (fakeExecutor) Cycle(_ context.Context, request model.Request) model.Operation {
	now := time.Now()
	return model.Operation{
		RunID: request.RunID, SampleID: request.SampleID, Phase: request.Phase,
		Concurrency: request.Concurrency, Worker: request.Worker,
		StartedAt: now, FinishedAt: now.Add(time.Microsecond),
		CycleAckNS: 1000, StartToReadyNS: 200, Status: model.StatusSuccess,
	}
}

type memoryWriter struct {
	mu      sync.Mutex
	records []model.Operation
}

func (w *memoryWriter) WriteOperation(operation model.Operation) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.records = append(w.records, operation)
	return nil
}

func TestRunProducesWarmupLatencyAndCapacityRecords(t *testing.T) {
	writer := &memoryWriter{}
	spec := campaign.Spec{
		Warmup:      campaign.Warmup{MinCycles: 2, MinDuration: campaign.Duration{Duration: time.Nanosecond}},
		Measurement: campaign.Measurement{Samples: 3, CycleTimeout: campaign.Duration{Duration: time.Second}},
		Capacity:    campaign.Capacity{Enabled: true, Workers: []int{2}, DurationPerPoint: campaign.Duration{Duration: time.Millisecond}},
	}
	runner := Runner{Spec: spec, RunID: "test", Executor: fakeExecutor{}, Writer: writer}
	records, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(records) < 6 {
		t.Fatalf("got %d records, want warmup + latency + capacity", len(records))
	}
	if records[0].Phase != model.PhaseWarmup || records[2].Phase != model.PhaseLatency {
		t.Fatalf("unexpected phase order: %#v", records[:3])
	}
}
