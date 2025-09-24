package worker

import "time"

// Status represents the lifecycle state of a worker instance.
type Status string

const (
	// StatusHealthy indicates the worker is actively heartbeating within the allowed window.
	StatusHealthy Status = "healthy"
	// StatusStale marks a worker whose heartbeat has exceeded the staleness threshold.
	StatusStale Status = "stale"
)

// Worker captures metadata about a registered worker node.
type Worker struct {
	ID              string    `json:"id"`
	Pool            string    `json:"pool"`
	Capabilities    []string  `json:"capabilities"`
	Endpoint        string    `json:"endpoint"`
	Version         string    `json:"version"`
	Status          Status    `json:"status"`
	RegisteredAt    time.Time `json:"registered_at"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
	ActiveJobs      int       `json:"active_jobs"`
}

// Assignment communicates queue metadata returned during registration.
type Assignment struct {
	Stream            string        `json:"stream"`
	ConsumerGroup     string        `json:"consumer_group"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
}

// RegistrationRequest captures worker supplied metadata during the handshake.
type RegistrationRequest struct {
	Pool         string
	Capabilities []string
	Endpoint     string
	Version      string
}

// RegistrationResult returns the persisted worker metadata and queue assignment details.
type RegistrationResult struct {
	Worker     Worker
	Assignment Assignment
}

// HeartbeatInput is provided by workers to renew their availability information.
type HeartbeatInput struct {
	WorkerID   string
	ActiveJobs int
}

// HeartbeatResult reports the worker state after processing a heartbeat.
type HeartbeatResult struct {
	Worker Worker
	Stale  bool
}

// PoolConfig captures tunable settings for a worker pool.
type PoolConfig struct {
	Pool              string        `json:"pool"`
	Description       string        `json:"description"`
	Stream            string        `json:"stream"`
	ConsumerGroup     string        `json:"consumer_group"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
	StaleAfter        time.Duration `json:"stale_after"`
	MaxConcurrentJobs int           `json:"max_concurrent_jobs"`
}
