package worker

import "errors"

var (
	// ErrInvalidPool indicates the worker did not supply a pool during registration.
	ErrInvalidPool = errors.New("worker: pool is required")
	// ErrInvalidWorkerID indicates a heartbeat was attempted without an identifier.
	ErrInvalidWorkerID = errors.New("worker: worker id is required")
	// ErrUnknownWorker is returned when a worker lookup fails.
	ErrUnknownWorker = errors.New("worker: worker not found")
)
