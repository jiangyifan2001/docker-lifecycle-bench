package summary

import (
	"sort"
	"strconv"
	"time"

	"docker-lifecycle-bench/internal/model"
)

type Percentiles struct {
	P50 uint64 `json:"p50"`
	P95 uint64 `json:"p95"`
	P99 uint64 `json:"p99"`
}

type Group struct {
	Phase               string      `json:"phase"`
	Concurrency         int         `json:"concurrency"`
	Attempts            int         `json:"attempts"`
	Successes           int         `json:"successes"`
	Errors              int         `json:"errors"`
	ErrorRate           float64     `json:"errorRate"`
	ObservedSeconds     float64     `json:"observedSeconds"`
	CompletedCyclesPerS float64     `json:"completedCyclesPerSecond"`
	CycleAckNS          Percentiles `json:"cycleAckNs"`
	StartToReadyNS      Percentiles `json:"startToReadyNs"`
}

type Summary struct {
	Classification string   `json:"classification"`
	Scenario       string   `json:"scenario"`
	Groups         []Group  `json:"groups"`
	Caveats        []string `json:"caveats"`
}

func Build(operations []model.Operation) Summary {
	groups := make(map[string][]model.Operation)
	for _, operation := range operations {
		key := operation.Phase + "/" + strconv.Itoa(operation.Concurrency)
		groups[key] = append(groups[key], operation)
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := Summary{
		Classification: "diagnostic",
		Scenario:       "full-lifecycle",
		Caveats: []string{
			"Phase 0 smoke result; not valid for cross-platform ranking.",
			"p99.9 is intentionally omitted because the smoke sample count is insufficient.",
		},
	}
	for _, key := range keys {
		result.Groups = append(result.Groups, summarizeGroup(groups[key]))
	}
	return result
}

func summarizeGroup(operations []model.Operation) Group {
	group := Group{
		Phase:       operations[0].Phase,
		Concurrency: operations[0].Concurrency,
		Attempts:    len(operations),
	}
	var cycleValues, readyValues []uint64
	var first, last time.Time
	for _, operation := range operations {
		if first.IsZero() || operation.StartedAt.Before(first) {
			first = operation.StartedAt
		}
		if operation.FinishedAt.After(last) {
			last = operation.FinishedAt
		}
		if operation.Status != model.StatusSuccess {
			group.Errors++
			continue
		}
		group.Successes++
		cycleValues = append(cycleValues, operation.CycleAckNS)
		readyValues = append(readyValues, operation.StartToReadyNS)
	}
	if group.Attempts > 0 {
		group.ErrorRate = float64(group.Errors) / float64(group.Attempts)
	}
	group.ObservedSeconds = last.Sub(first).Seconds()
	if group.ObservedSeconds > 0 {
		group.CompletedCyclesPerS = float64(group.Successes) / group.ObservedSeconds
	}
	group.CycleAckNS = percentiles(cycleValues)
	group.StartToReadyNS = percentiles(readyValues)
	return group
}

func percentiles(values []uint64) Percentiles {
	if len(values) == 0 {
		return Percentiles{}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return Percentiles{
		P50: nearestRank(values, 0.50),
		P95: nearestRank(values, 0.95),
		P99: nearestRank(values, 0.99),
	}
}

func nearestRank(sorted []uint64, quantile float64) uint64 {
	index := int(float64(len(sorted))*quantile+0.999999999) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}
