// Package model holds the shared domain types used across api, scheduler, worker and store.
// It intentionally has zero dependencies on the other internal packages so everything else
// can depend on it without import cycles.
package model

import "time"

type JobStatus string

const (
	JobStatusActive   JobStatus = "active"
	JobStatusPaused   JobStatus = "paused"
	JobStatusArchived JobStatus = "archived"
)

type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusLeased    RunStatus = "leased"
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusDead      RunStatus = "dead"
)

// Job is a job definition: either a one-shot task or a recurring (cron_expr set) template.
// Each time a Job becomes eligible to run, a JobRun is created for it.
type Job struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Payload         map[string]any  `json:"payload"`
	CronExpr        *string         `json:"cron_expr,omitempty"`
	Priority        int16           `json:"priority"`
	MaxAttempts     int16           `json:"max_attempts"`
	TimeoutSeconds  int32           `json:"timeout_seconds"`
	Status          JobStatus       `json:"status"`
	IdempotencyKey  *string         `json:"idempotency_key,omitempty"`
	DependsOn       []string        `json:"depends_on,omitempty"` // job IDs this job's runs must wait on
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// JobRun is a single execution attempt (or series of attempts) of a Job.
type JobRun struct {
	ID             string     `json:"id"`
	JobID          string     `json:"job_id"`
	Status         RunStatus  `json:"status"`
	Attempt        int16      `json:"attempt"`
	Priority       int16      `json:"priority"`
	ScheduledAt    time.Time  `json:"scheduled_at"`
	LeasedBy       *string    `json:"leased_by,omitempty"`
	LeasedAt       *time.Time `json:"leased_at,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	Result         map[string]any `json:"result,omitempty"`
	Error          *string    `json:"error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type WorkerStatus string

const (
	WorkerStatusAlive WorkerStatus = "alive"
	WorkerStatusDead  WorkerStatus = "dead"
)

type Worker struct {
	ID            string       `json:"id"`
	Hostname      string       `json:"hostname"`
	Status        WorkerStatus `json:"status"`
	LastHeartbeat time.Time    `json:"last_heartbeat"`
	StartedAt     time.Time    `json:"started_at"`
}

type DeadLetter struct {
	ID        string         `json:"id"`
	JobRunID  string         `json:"job_run_id"`
	Reason    string         `json:"reason"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// NewJobInput / NewRunInput are the create-shape payloads used by the API and scheduler,
// kept distinct from the persisted Job/JobRun so server-assigned fields (ID, timestamps)
// can't be forged by a caller.
type NewJobInput struct {
	Name            string         `json:"name"`
	Payload         map[string]any `json:"payload"`
	CronExpr        *string        `json:"cron_expr,omitempty"`
	Priority        int16          `json:"priority"`
	MaxAttempts     int16          `json:"max_attempts"`
	TimeoutSeconds  int32          `json:"timeout_seconds"`
	IdempotencyKey  *string        `json:"idempotency_key,omitempty"`
	DependsOn       []string       `json:"depends_on,omitempty"`
}
