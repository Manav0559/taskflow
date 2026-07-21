package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/manavsingla/taskflow/internal/model"
)

const testSecret = "handler-test-secret"

func newTestRouter(t *testing.T) (http.Handler, *fakeStore, string) {
	t.Helper()
	st := newFakeStore()
	log := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	// High rate limit so validation tests aren't incidentally rate-limited.
	router := NewRouter(st, log, testSecret, 10000, 10000)
	token, err := MintToken(testSecret, "handler-test", time.Hour)
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}
	return router, st, token
}

func doRequest(t *testing.T, router http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestCreateJob_Validation(t *testing.T) {
	router, _, token := newTestRouter(t)

	tests := []struct {
		name       string
		body       map[string]any
		wantStatus int
	}{
		{"missing name", map[string]any{"payload": map[string]any{}}, http.StatusBadRequest},
		{"invalid cron_expr", map[string]any{"name": "echo", "cron_expr": "not a cron"}, http.StatusBadRequest},
		{"max_attempts zero defaults, is valid", map[string]any{"name": "echo", "max_attempts": 0}, http.StatusCreated},
		{"negative max_attempts rejected", map[string]any{"name": "echo", "max_attempts": -1}, http.StatusBadRequest},
		{"negative timeout_seconds rejected", map[string]any{"name": "echo", "timeout_seconds": -5}, http.StatusBadRequest},
		{"valid minimal job", map[string]any{"name": "echo"}, http.StatusCreated},
		{"valid cron job", map[string]any{"name": "echo", "cron_expr": "*/5 * * * *"}, http.StatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(t, router, http.MethodPost, "/v1/jobs", token, tt.body)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestCreateJob_Defaults(t *testing.T) {
	router, _, token := newTestRouter(t)

	rec := doRequest(t, router, http.MethodPost, "/v1/jobs", token, map[string]any{"name": "echo"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	var job model.Job
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if job.MaxAttempts != 5 {
		t.Errorf("max_attempts default = %d, want 5", job.MaxAttempts)
	}
	if job.TimeoutSeconds != 300 {
		t.Errorf("timeout_seconds default = %d, want 300", job.TimeoutSeconds)
	}
}

func TestCreateJob_Idempotency(t *testing.T) {
	router, _, token := newTestRouter(t)
	body := map[string]any{"name": "echo", "idempotency_key": "dup-key-1"}

	rec1 := doRequest(t, router, http.MethodPost, "/v1/jobs", token, body)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first create: status = %d, want 201 (body: %s)", rec1.Code, rec1.Body.String())
	}
	var job1 model.Job
	if err := json.Unmarshal(rec1.Body.Bytes(), &job1); err != nil {
		t.Fatalf("unmarshal first response: %v", err)
	}

	rec2 := doRequest(t, router, http.MethodPost, "/v1/jobs", token, body)
	if rec2.Code != http.StatusOK {
		t.Fatalf("idempotent replay: status = %d, want 200 (body: %s)", rec2.Code, rec2.Body.String())
	}
	var job2 model.Job
	if err := json.Unmarshal(rec2.Body.Bytes(), &job2); err != nil {
		t.Fatalf("unmarshal second response: %v", err)
	}
	if job1.ID != job2.ID {
		t.Errorf("idempotent replay created a new job: got %s, want %s", job2.ID, job1.ID)
	}
}

func TestAuth_RequiredOnV1Routes(t *testing.T) {
	router, _, _ := newTestRouter(t)

	paths := []string{"/v1/jobs", "/v1/workers"}
	for _, p := range paths {
		rec := doRequest(t, router, http.MethodGet, p, "" /* no token */, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without token: status = %d, want 401", p, rec.Code)
		}
	}
}

func TestHealthzAndMetrics_NoAuthRequired(t *testing.T) {
	router, _, _ := newTestRouter(t)

	rec := doRequest(t, router, http.MethodGet, "/healthz", "", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /healthz: status = %d, want 200", rec.Code)
	}

	rec = doRequest(t, router, http.MethodGet, "/metrics", "", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /metrics: status = %d, want 200", rec.Code)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	router, _, token := newTestRouter(t)
	rec := doRequest(t, router, http.MethodGet, "/v1/jobs/does-not-exist", token, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestPauseResumeJob(t *testing.T) {
	router, _, token := newTestRouter(t)

	rec := doRequest(t, router, http.MethodPost, "/v1/jobs", token, map[string]any{"name": "echo"})
	var job model.Job
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	rec = doRequest(t, router, http.MethodPost, "/v1/jobs/"+job.ID+"/pause", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("pause: status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var paused model.Job
	if err := json.Unmarshal(rec.Body.Bytes(), &paused); err != nil {
		t.Fatalf("unmarshal pause response: %v", err)
	}
	if paused.Status != model.JobStatusPaused {
		t.Errorf("status after pause = %q, want %q", paused.Status, model.JobStatusPaused)
	}

	rec = doRequest(t, router, http.MethodPost, "/v1/jobs/"+job.ID+"/resume", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("resume: status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resumed model.Job
	if err := json.Unmarshal(rec.Body.Bytes(), &resumed); err != nil {
		t.Fatalf("unmarshal resume response: %v", err)
	}
	if resumed.Status != model.JobStatusActive {
		t.Errorf("status after resume = %q, want %q", resumed.Status, model.JobStatusActive)
	}
}

func TestListJobs_InvalidStatusFilter(t *testing.T) {
	router, _, token := newTestRouter(t)
	rec := doRequest(t, router, http.MethodGet, "/v1/jobs?status=not-a-real-status", token, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestListJobs_InvalidLimit(t *testing.T) {
	router, _, token := newTestRouter(t)
	rec := doRequest(t, router, http.MethodGet, "/v1/jobs?limit=not-a-number", token, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
}
