package model

import "time"

const (
	PhaseWarmup   = "warmup"
	PhaseLatency  = "latency"
	PhaseCapacity = "capacity"

	StatusSuccess = "success"
	StatusError   = "error"
)

type Operation struct {
	RunID            string    `json:"runId"`
	SampleID         uint64    `json:"sampleId"`
	Phase            string    `json:"phase"`
	Concurrency      int       `json:"concurrency"`
	Worker           int       `json:"worker"`
	ContainerID      string    `json:"containerId,omitempty"`
	StartedAt        time.Time `json:"startedAt"`
	FinishedAt       time.Time `json:"finishedAt"`
	CreateNS         uint64    `json:"createNs,omitempty"`
	StartAPINS       uint64    `json:"startApiNs,omitempty"`
	StartToReadyNS   uint64    `json:"startToReadyNs,omitempty"`
	ReadyObservedNS  uint64    `json:"readyObservedNs,omitempty"`
	KillAPINS        uint64    `json:"killApiNs,omitempty"`
	DestroyAckNS     uint64    `json:"destroyAckNs,omitempty"`
	RemoveAckNS      uint64    `json:"removeAckNs,omitempty"`
	CycleAckNS       uint64    `json:"cycleAckNs,omitempty"`
	ProbeMonotonicNS uint64    `json:"probeMonotonicNs,omitempty"`
	ProbePID         uint32    `json:"probePid,omitempty"`
	Status           string    `json:"status"`
	ErrorStage       string    `json:"errorStage,omitempty"`
	Error            string    `json:"error,omitempty"`
}

type Request struct {
	RunID       string
	SampleID    uint64
	Phase       string
	Concurrency int
	Worker      int
}
