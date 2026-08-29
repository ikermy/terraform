package entity

import "errors"

// Domain-level sentinel errors. They describe business rules and are
// independent of any transport (HTTP/gRPC), so usecase and delivery depend
// only on these, never on infrastructure error types.
var (
	ErrClusterNotFound     = errors.New("cluster not found")
	ErrClusterNameRequired = errors.New("cluster name is required")
	ErrClusterNegativeReps = errors.New("cluster replicas cannot be negative")
	ErrClusterIDRequired   = errors.New("cluster id is required")
	ErrClusterConflict     = errors.New("cluster already exists")

	ErrJobNotFound     = errors.New("job not found")
	ErrJobNameRequired = errors.New("job name is required")
	ErrJobNegativePri  = errors.New("job priority cannot be negative")
	ErrJobIDRequired   = errors.New("job id is required")
)
