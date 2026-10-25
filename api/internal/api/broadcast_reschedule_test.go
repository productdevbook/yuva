package api_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
	"uuid"
)

type startJob struct {
	id    int64
	state string
	at    time.Time
}

func startJobs(t *testing.T, h *harness, ws, id string) []startJob {
	t.Helper()
	rows, err := h.st.Pool.Query(context.Background(), `SELECT id, state::text, scheduled_at FROM river_job
		WHERE kind = 'broadcast_start' AND args->>'workspace_id' = $1 AND args->>'broadcast_id' = $2 ORDER BY id`, ws, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []startJob
	for rows.Next() {
		var j startJob
		if err := rows.Scan(&j.id, &j.state, &j.at); err != nil {
			t.Fatal(err)
		}
		out = append(out, j)
	}
	return out
}

func markStartJobRunning(t *testing.T, h *harness, ws, id string) {
	t.Helper()
	if _, err := h.st.Pool.Exec(context.Background(), `UPDATE river_job SET state = 'running', attempted_at = now(), attempt = 1
		WHERE kind = 'broadcast_start' AND args->>'workspace_id' = $1 AND args->>'broadcast_id' = $2`, ws, id); err != nil {
		t.Fatal(err)
	}
}

func scheduleAt(bt broadcastTeam, id string, at time.Time) {
	bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/schedule", map[string]any{"send_at": at.Format(time.RFC3339)})
}

func TestBroadcastRescheduleEarlier(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	setClock(h.clock, time.Now().UTC().Truncate(time.Second))
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "early@example.org")
	id := bt.draft(t, nil)
	later := h.clock.Now().Add(48 * time.Hour)
	scheduleAt(bt, id, later)
	if jobs := startJobs(t, h, bt.ws, id); len(jobs) != 1 || !jobs[0].at.Equal(later) {
		t.Fatalf("start jobs after scheduling for later: %+v", jobs)
	}
	bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/unschedule", nil)
	if jobs := startJobs(t, h, bt.ws, id); len(jobs) != 0 {
		t.Fatalf("start jobs left after going back to draft: %+v", jobs)
	}
	bt.schedule(t, id)
	if jobs := startJobs(t, h, bt.ws, id); len(jobs) != 1 || jobs[0].at.After(h.clock.Now()) {
		t.Fatalf("start jobs after scheduling now: %+v, now %v", jobs, h.clock.Now())
	}
	bt.run(t)
	if b := bt.get(t, id); b["state"] != "sent" || bs.count() != 1 {
		t.Fatalf("scheduled now, after a later time was taken back: %d mails, %v", bs.count(), b["state"])
	}
}

func TestBroadcastRescheduleWhileStartJobRuns(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	setClock(h.clock, time.Now().UTC().Truncate(time.Second))
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "running@example.org")
	id := bt.draft(t, nil)
	scheduleAt(bt, id, h.clock.Now().Add(48*time.Hour))
	markStartJobRunning(t, h, bt.ws, id)
	bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/unschedule", nil)
	scheduleAt(bt, id, h.clock.Now().Add(time.Hour))
	if jobs := startJobs(t, h, bt.ws, id); len(jobs) != 1 || jobs[0].state != "running" {
		t.Fatalf("start jobs after rescheduling while one runs: %+v", jobs)
	}
	ctx := context.Background()
	ws, bid := uuid.MustParse(bt.ws), uuid.MustParse(id)
	start, snoozes := h.clock.Now(), 0
	for {
		wait, err := h.srv.StartBroadcast(ctx, ws, bid, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if wait < 0 {
			break
		}
		if wait != 5*time.Minute || snoozes == 12 {
			t.Fatalf("snooze %d of the running job: %v, want 5m until the new time", snoozes+1, wait)
		}
		snoozes++
		h.clock.Advance(wait)
	}
	if h.clock.Now().Sub(start) != time.Hour {
		t.Fatalf("the running job started the broadcast %v after rescheduling, want 1h", h.clock.Now().Sub(start))
	}
	bt.run(t)
	if b := bt.get(t, id); b["state"] != "sent" || bs.count() != 1 {
		t.Fatalf("rescheduled while a start job ran: %d mails, %v", bs.count(), b["state"])
	}
}

func TestBroadcastTwoStartJobsSnapshotOnce(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	const n = 2500
	bt.subscribeMany(t, "snap", n)
	id := bt.draft(t, nil)
	bt.schedule(t, id)
	ctx := context.Background()
	ws, bid := uuid.MustParse(bt.ws), uuid.MustParse(id)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Go(func() {
			for {
				wait, err := h.srv.StartBroadcast(ctx, ws, bid, 0)
				if err != nil || wait != 0 {
					errs[i] = err
					return
				}
			}
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var rows, contacts int
	if err := h.st.Pool.QueryRow(ctx, `SELECT count(*), count(DISTINCT contact_id) FROM broadcast_deliveries WHERE workspace_id = $1 AND broadcast_id = $2`, ws, bid).Scan(&rows, &contacts); err != nil {
		t.Fatal(err)
	}
	b := bt.get(t, id)
	if rows != n || contacts != n || b["state"] != "sending" || counts(b)["recipients"] != float64(n) {
		t.Fatalf("two snapshots at once: %d deliveries to %d contacts, %v, counts %v", rows, contacts, b["state"], counts(b))
	}
}

func TestBroadcastCancelDropsStartJob(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "cancel@example.org")
	id := bt.draft(t, nil)
	scheduleAt(bt, id, h.clock.Now().Add(time.Hour))
	if jobs := startJobs(t, h, bt.ws, id); len(jobs) != 1 {
		t.Fatalf("start jobs after scheduling: %+v", jobs)
	}
	bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/cancel", nil)
	if jobs := startJobs(t, h, bt.ws, id); len(jobs) != 0 {
		t.Fatalf("start jobs left after cancelling: %+v", jobs)
	}
}
