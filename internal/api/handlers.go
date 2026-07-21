package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/robfig/cron/v3"

	"github.com/manavsingla/taskflow/internal/metrics"
	"github.com/manavsingla/taskflow/internal/model"
	"github.com/manavsingla/taskflow/internal/store"
)

const (
	maxCreateJobBodyBytes = 64 * 1024
	defaultListLimit      = 50
	maxListLimit          = 200
)

type handler struct {
	store store.Store
	log   *slog.Logger
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// serverError logs the real error server-side and returns a generic body to the
// client — the caller never learns internal details (query text, driver errors, etc.)
// from a 500.
func (h *handler) serverError(w http.ResponseWriter, err error, context string) {
	h.log.Error(context, "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func (h *handler) createJob(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCreateJobBodyBytes)

	var in model.NewJobInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(in.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if in.CronExpr != nil {
		if _, err := cron.ParseStandard(*in.CronExpr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid cron_expr")
			return
		}
	}

	switch {
	case in.MaxAttempts == 0:
		in.MaxAttempts = 5
	case in.MaxAttempts < 1:
		writeError(w, http.StatusBadRequest, "max_attempts must be >= 1")
		return
	}

	switch {
	case in.TimeoutSeconds == 0:
		in.TimeoutSeconds = 300
	case in.TimeoutSeconds < 1:
		writeError(w, http.StatusBadRequest, "timeout_seconds must be >= 1")
		return
	}

	if in.IdempotencyKey != nil && *in.IdempotencyKey != "" {
		existing, err := h.store.GetJobByIdempotencyKey(r.Context(), *in.IdempotencyKey)
		if err == nil {
			writeJSON(w, http.StatusOK, existing)
			return
		}
		if !errors.Is(err, store.ErrNotFound) {
			h.serverError(w, err, "GetJobByIdempotencyKey failed")
			return
		}
	}

	job, err := h.store.CreateJob(r.Context(), in)
	if err != nil {
		if errors.Is(err, store.ErrIdempotencyConflict) {
			writeError(w, http.StatusConflict, "idempotency key already used")
			return
		}
		h.serverError(w, err, "CreateJob failed")
		return
	}

	metrics.JobsSubmitted.Inc()
	writeJSON(w, http.StatusCreated, job)
}

func (h *handler) listJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var statusPtr *model.JobStatus
	if s := q.Get("status"); s != "" {
		st := model.JobStatus(s)
		switch st {
		case model.JobStatusActive, model.JobStatusPaused, model.JobStatusArchived:
			statusPtr = &st
		default:
			writeError(w, http.StatusBadRequest, "invalid status")
			return
		}
	}

	limit, ok := parseLimit(q.Get("limit"), w)
	if !ok {
		return
	}
	offset, ok := parseOffset(q.Get("offset"), w)
	if !ok {
		return
	}

	jobs, err := h.store.ListJobs(r.Context(), statusPtr, limit, offset)
	if err != nil {
		h.serverError(w, err, "ListJobs failed")
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (h *handler) getJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.store.GetJob(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		h.serverError(w, err, "GetJob failed")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *handler) pauseJob(w http.ResponseWriter, r *http.Request) {
	h.updateJobStatus(w, r, model.JobStatusPaused)
}

func (h *handler) resumeJob(w http.ResponseWriter, r *http.Request) {
	h.updateJobStatus(w, r, model.JobStatusActive)
}

func (h *handler) updateJobStatus(w http.ResponseWriter, r *http.Request, status model.JobStatus) {
	id := chi.URLParam(r, "id")
	if err := h.store.UpdateJobStatus(r.Context(), id, status); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		h.serverError(w, err, "UpdateJobStatus failed")
		return
	}

	job, err := h.store.GetJob(r.Context(), id)
	if err != nil {
		h.serverError(w, err, "GetJob failed")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *handler) listJobRuns(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	limit, ok := parseLimit(r.URL.Query().Get("limit"), w)
	if !ok {
		return
	}

	runs, err := h.store.ListJobRuns(r.Context(), id, limit)
	if err != nil {
		h.serverError(w, err, "ListJobRuns failed")
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *handler) getRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	run, err := h.store.GetRun(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		h.serverError(w, err, "GetRun failed")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (h *handler) listWorkers(w http.ResponseWriter, r *http.Request) {
	workers, err := h.store.ListWorkers(r.Context())
	if err != nil {
		h.serverError(w, err, "ListWorkers failed")
		return
	}
	writeJSON(w, http.StatusOK, workers)
}

func parseLimit(raw string, w http.ResponseWriter) (int, bool) {
	limit := defaultListLimit
	if raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return 0, false
		}
		limit = v
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	return limit, true
}

func parseOffset(raw string, w http.ResponseWriter) (int, bool) {
	if raw == "" {
		return 0, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		writeError(w, http.StatusBadRequest, "invalid offset")
		return 0, false
	}
	return v, true
}
