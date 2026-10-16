package summary

import (
	"testing"
	"time"

	"docker-lifecycle-bench/internal/model"
)

func TestBuildSeparatesErrorsFromLatencyDistribution(t *testing.T) {
	start := time.Unix(100, 0)
	operations := []model.Operation{
		{Phase: model.PhaseLatency, Concurrency: 1, Status: model.StatusSuccess, CycleAckNS: 10, StartToReadyNS: 3, StartedAt: start, FinishedAt: start.Add(time.Second)},
		{Phase: model.PhaseLatency, Concurrency: 1, Status: model.StatusError, CycleAckNS: 9999, StartedAt: start.Add(time.Second), FinishedAt: start.Add(2 * time.Second)},
		{Phase: model.PhaseLatency, Concurrency: 1, Status: model.StatusSuccess, CycleAckNS: 20, StartToReadyNS: 7, StartedAt: start.Add(2 * time.Second), FinishedAt: start.Add(3 * time.Second)},
	}

	got := Build(operations)
	group := got.Groups[0]
	if group.Attempts != 3 || group.Successes != 2 || group.Errors != 1 {
		t.Fatalf("group counts = %+v", group)
	}
	if group.CycleAckNS.P99 != 20 {
		t.Fatalf("p99 = %d, failure latency leaked into distribution", group.CycleAckNS.P99)
	}
}
