package integration

import (
	"context"
	"errors"
	"math"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/manavsingla/taskflow/internal/model"
	"github.com/manavsingla/taskflow/internal/store"
)

// shortLease is long enough for the test's own writes to land inside it but short
// enough that waiting it out keeps the test fast.
const shortLease = 300 * time.Millisecond

func openStore(t *testing.T) (context.Context, *store.PostgresStore) {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test (requires a real Postgres)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	st, err := store.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	t.Cleanup(st.Close)

	if err := store.RunMigrations(ctx, st.Pool(), "../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return ctx, st
}

// newLeasableRun creates a job plus one pending run with the maximum priority, so the
// next LeaseNextRun claims it ahead of anything else left in a shared database. The
// job is archived on cleanup so later PromoteOnce calls (in this or a rerun of the
// suite) never pick it up.
func newLeasableRun(t *testing.T, ctx context.Context, st *store.PostgresStore) *model.JobRun {
	t.Helper()
	job, err := st.CreateJob(ctx, model.NewJobInput{
		Name:           "fencing-" + t.Name(),
		Payload:        map[string]any{},
		Priority:       math.MaxInt16,
		MaxAttempts:    5,
		TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	t.Cleanup(func() {
		_ = st.UpdateJobStatus(context.Background(), job.ID, model.JobStatusArchived)
	})

	run, err := st.CreateRun(ctx, job.ID, math.MaxInt16, time.Now().Add(-time.Second))
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	return run
}

func mustLease(t *testing.T, ctx context.Context, st *store.PostgresStore, workerID, wantRunID string) *model.JobRun {
	t.Helper()
	run, _, err := st.LeaseNextRun(ctx, workerID, shortLease)
	if err != nil {
		t.Fatalf("%s: LeaseNextRun: %v", workerID, err)
	}
	if run == nil {
		t.Fatalf("%s: LeaseNextRun returned no run", workerID)
	}
	if run.ID != wantRunID {
		t.Fatalf("%s: leased run %s, want %s", workerID, run.ID, wantRunID)
	}
	return run
}

// expireAndReclaim waits out the lease on runID and runs the janitor until the run is
// back to pending, mirroring what a live worker's janitorLoop does.
func expireAndReclaim(t *testing.T, ctx context.Context, st *store.PostgresStore, runID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(shortLease / 2)
		if _, err := st.ReclaimExpiredLeases(ctx); err != nil {
			t.Fatalf("ReclaimExpiredLeases: %v", err)
		}
		run, err := st.GetRun(ctx, runID)
		if err != nil {
			t.Fatalf("GetRun: %v", err)
		}
		if run.Status == model.RunStatusPending {
			return
		}
	}
	t.Fatalf("run %s was not reclaimed within 5s", runID)
}

func wantLeaseLost(t *testing.T, op string, err error) {
	t.Helper()
	if !errors.Is(err, store.ErrLeaseLost) {
		t.Errorf("stale %s: got %v, want store.ErrLeaseLost", op, err)
	}
}

// TestStaleWorkerCannotOverwriteReleasedRun is the lost-update scenario: worker A's
// lease expires mid-run, the janitor reclaims the run and worker B leases it. Every
// write A attempts afterwards must be rejected, and the row must reflect only B.
func TestStaleWorkerCannotOverwriteReleasedRun(t *testing.T) {
	ctx, st := openStore(t)
	created := newLeasableRun(t, ctx, st)

	runA := mustLease(t, ctx, st, "worker-a", created.ID)
	leaseA := store.LeaseOf(runA, "worker-a")
	if err := st.MarkRunning(ctx, leaseA); err != nil {
		t.Fatalf("worker-a MarkRunning: %v", err)
	}

	// A stalls (GC pause, partition, slow handler) past its lease.
	expireAndReclaim(t, ctx, st, created.ID)

	runB := mustLease(t, ctx, st, "worker-b", created.ID)
	leaseB := store.LeaseOf(runB, "worker-b")
	if runB.Attempt != runA.Attempt+1 {
		t.Fatalf("re-lease attempt = %d, want %d", runB.Attempt, runA.Attempt+1)
	}
	if err := st.MarkRunning(ctx, leaseB); err != nil {
		t.Fatalf("worker-b MarkRunning: %v", err)
	}

	// A wakes up and tries every transition while B holds the lease.
	wantLeaseLost(t, "ExtendLease", st.ExtendLease(ctx, leaseA, time.Minute))
	wantLeaseLost(t, "MarkRunning", st.MarkRunning(ctx, leaseA))
	wantLeaseLost(t, "CompleteRun", st.CompleteRun(ctx, leaseA, map[string]any{"by": "worker-a"}))
	wantLeaseLost(t, "FailRun(requeue)", st.FailRun(ctx, leaseA, "a failed", true, time.Second))
	wantLeaseLost(t, "FailRun(final)", st.FailRun(ctx, leaseA, "a failed", false, 0))
	wantLeaseLost(t, "MarkDead", st.MarkDead(ctx, leaseA, "a gave up"))

	mid, err := st.GetRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if mid.Status != model.RunStatusRunning || mid.LeasedBy == nil || *mid.LeasedBy != "worker-b" || mid.Error != nil {
		t.Fatalf("stale writes leaked into the row: status=%s leased_by=%v error=%v", mid.Status, mid.LeasedBy, mid.Error)
	}

	if err := st.CompleteRun(ctx, leaseB, map[string]any{"by": "worker-b"}); err != nil {
		t.Fatalf("worker-b CompleteRun: %v", err)
	}

	// A late completion from A after B has finished is still rejected.
	wantLeaseLost(t, "CompleteRun after B finished", st.CompleteRun(ctx, leaseA, map[string]any{"by": "worker-a"}))
	// And B cannot complete the same lease twice.
	wantLeaseLost(t, "duplicate CompleteRun", st.CompleteRun(ctx, leaseB, map[string]any{"by": "worker-b-again"}))

	final, err := st.GetRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if final.Status != model.RunStatusSucceeded {
		t.Errorf("status = %s, want succeeded", final.Status)
	}
	if final.LeasedBy == nil || *final.LeasedBy != "worker-b" {
		t.Errorf("leased_by = %v, want worker-b", final.LeasedBy)
	}
	if final.Attempt != runB.Attempt {
		t.Errorf("attempt = %d, want %d", final.Attempt, runB.Attempt)
	}
	if got := final.Result["by"]; got != "worker-b" {
		t.Errorf("result.by = %v, want worker-b", got)
	}
	if final.Error != nil {
		t.Errorf("error = %q, want nil (A's failure must not be recorded)", *final.Error)
	}
}

// TestAttemptFencesSameWorkerID covers a re-lease by the same worker ID (e.g. another
// goroutine of the same process, or a restarted pod reusing its name): leased_by
// alone would match, so attempt is what rejects the stale write.
func TestAttemptFencesSameWorkerID(t *testing.T) {
	ctx, st := openStore(t)
	created := newLeasableRun(t, ctx, st)

	first := mustLease(t, ctx, st, "worker-same", created.ID)
	stale := store.LeaseOf(first, "worker-same")

	expireAndReclaim(t, ctx, st, created.ID)

	second := mustLease(t, ctx, st, "worker-same", created.ID)
	current := store.LeaseOf(second, "worker-same")

	wantLeaseLost(t, "FailRun from earlier attempt", st.FailRun(ctx, stale, "stale", true, time.Second))
	if err := st.CompleteRun(ctx, current, map[string]any{"ok": true}); err != nil {
		t.Fatalf("current CompleteRun: %v", err)
	}

	final, err := st.GetRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if final.Status != model.RunStatusSucceeded || final.Error != nil {
		t.Errorf("row = status %s error %v, want succeeded with no error", final.Status, final.Error)
	}
}

// TestFencedWriteOnMissingRunIsNotFound checks that ErrLeaseLost is reserved for runs
// that exist but are held by someone else.
func TestFencedWriteOnMissingRunIsNotFound(t *testing.T) {
	ctx, st := openStore(t)
	lease := store.Lease{RunID: uuid.NewString(), WorkerID: "worker-x", Attempt: 1}
	if err := st.CompleteRun(ctx, lease, nil); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("CompleteRun on missing run: got %v, want store.ErrNotFound", err)
	}
	if err := st.MarkDead(ctx, lease, "x"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("MarkDead on missing run: got %v, want store.ErrNotFound", err)
	}
}

// TestFencedFailThenMarkDead checks the normal dead-letter path still works under
// fencing: FailRun(final) then MarkDead with the same lease.
func TestFencedFailThenMarkDead(t *testing.T) {
	ctx, st := openStore(t)
	created := newLeasableRun(t, ctx, st)

	run := mustLease(t, ctx, st, "worker-d", created.ID)
	lease := store.LeaseOf(run, "worker-d")
	if err := st.FailRun(ctx, lease, "boom", false, 0); err != nil {
		t.Fatalf("FailRun: %v", err)
	}
	if err := st.MarkDead(ctx, lease, "max attempts exceeded"); err != nil {
		t.Fatalf("MarkDead: %v", err)
	}
	// A second MarkDead (e.g. a retried call) must not insert another dead letter.
	wantLeaseLost(t, "second MarkDead", st.MarkDead(ctx, lease, "again"))

	var letters int
	if err := st.Pool().QueryRow(ctx, `SELECT count(*) FROM dead_letters WHERE job_run_id = $1`, created.ID).Scan(&letters); err != nil {
		t.Fatalf("count dead letters: %v", err)
	}
	if letters != 1 {
		t.Errorf("dead_letters rows = %d, want 1", letters)
	}
	final, err := st.GetRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if final.Status != model.RunStatusDead {
		t.Errorf("status = %s, want dead", final.Status)
	}
}
