package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.icco.me/gotak"
	"go.icco.me/gotak/ai"
	"go.uber.org/zap"
)

func TestClaimAnalysisJob(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now()
	key := cacheKey(7, "advanced", "balanced", 2e9, "v2:moves=40")

	first, start, err := claimAnalysisJob(db, key, time.Minute, now)
	if err != nil || !start || first.Status != JobRunning {
		t.Fatalf("new job: job=%+v start=%v err=%v, want a running job to start", first, start, err)
	}

	again, start, err := claimAnalysisJob(db, key, time.Minute, now.Add(time.Second))
	if err != nil || start || again.ID != first.ID {
		t.Fatalf("running job: job=%+v start=%v err=%v, want the same job and no start", again, start, err)
	}

	stale, start, err := claimAnalysisJob(db, key, time.Minute, now.Add(time.Hour))
	if err != nil || !start || stale.ID != first.ID || stale.Attempt != first.Attempt+1 {
		t.Fatalf("abandoned job: job=%+v start=%v err=%v, want it reclaimed as the next attempt", stale, start, err)
	}

	for _, status := range []string{JobFailed, JobDone} {
		if err := db.Model(&AnalysisJob{}).Where("id = ?", first.ID).Update("status", status).Error; err != nil {
			t.Fatal(err)
		}
		job, start, err := claimAnalysisJob(db, key, time.Minute, now.Add(time.Hour))
		if err != nil || !start || job.Status != JobRunning {
			t.Errorf("%s job without a cached result: job=%+v start=%v err=%v, want a restart", status, job, start, err)
		}
	}

	other, start, err := claimAnalysisJob(db, cacheKey(7, "expert", "balanced", 2e9, "v2:moves=40"), time.Minute, now)
	if err != nil || !start || other.ID == first.ID {
		t.Errorf("different key: job=%+v start=%v err=%v, want a separate job", other, start, err)
	}
}

func TestRunAnalysisJob(t *testing.T) {
	g := playGame(t, []scriptedMove{
		{gotak.PlayerWhite, "a1"},
		{gotak.PlayerBlack, "e5"},
	})
	l := zap.NewNop().Sugar()

	t.Run("done", func(t *testing.T) {
		db := setupTestDB(t)
		key := cacheKey(g.ID, "advanced", "balanced", 2e9, gameCacheVersion(g))
		job, _, err := claimAnalysisJob(db, key, time.Minute, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		ev := okEval()
		runAnalysisJob(context.Background(), db, l, job, key, evFn(ev), g, time.Minute)

		var got AnalysisJob
		if err := db.First(&got, job.ID).Error; err != nil {
			t.Fatal(err)
		}
		if got.Status != JobDone || got.Error != "" {
			t.Fatalf("job = %+v, want done", got)
		}
		resp := jobResponse(db, l, &got, "alice-vs-bob", 5, time.Now())
		if resp.Result == nil || resp.Result.MoveCount != 2 || resp.Result.Slug != "alice-vs-bob" {
			t.Errorf("result = %+v, want the cached two-move analysis", resp.Result)
		}
		if resp.StatusURL == "" {
			t.Error("want a status URL")
		}
	})

	t.Run("failed", func(t *testing.T) {
		db := setupTestDB(t)
		key := cacheKey(g.ID, "advanced", "balanced", 2e9, gameCacheVersion(g))
		job, _, err := claimAnalysisJob(db, key, time.Minute, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		runAnalysisJob(context.Background(), db, l, job, key, evFn(&stubEvaluator{}), g, time.Minute)

		var got AnalysisJob
		if err := db.First(&got, job.ID).Error; err != nil {
			t.Fatal(err)
		}
		if got.Status != JobFailed || got.Error == "" {
			t.Errorf("job = %+v, want failed with an error", got)
		}
		if _, ok := loadAnalysisCache(db, l, key); ok {
			t.Error("a failed run must not be cached")
		}
	})

	t.Run("superseded attempt does not overwrite", func(t *testing.T) {
		db := setupTestDB(t)
		key := cacheKey(g.ID, "advanced", "balanced", 2e9, gameCacheVersion(g))
		old, _, err := claimAnalysisJob(db, key, time.Minute, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := claimAnalysisJob(db, key, time.Minute, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		runAnalysisJob(context.Background(), db, l, old, key, evFn(okEval()), g, time.Minute)

		var got AnalysisJob
		if err := db.First(&got, old.ID).Error; err != nil {
			t.Fatal(err)
		}
		if got.Status != JobRunning {
			t.Errorf("status = %q, want the newer attempt still running", got.Status)
		}
	})
}

func TestJobResponse_doneWithoutCache(t *testing.T) {
	db := setupTestDB(t)
	job := &AnalysisJob{ID: 3, GameID: 9, Level: "advanced", Style: "balanced", TimeLimitNs: 2e9, GameVersion: "v2:moves=1", Status: JobDone}
	resp := jobResponse(db, zap.NewNop().Sugar(), job, "alice-vs-bob", 5, time.Now())
	if resp.Status != JobFailed || resp.Error == "" || resp.Result != nil {
		t.Errorf("resp = %+v, want failed when the cached result is gone", resp)
	}
}

func evFn(ev positionEvaluator) func() (positionEvaluator, error) {
	return func() (positionEvaluator, error) { return ev, nil }
}

func okEval() *stubEvaluator {
	return &stubEvaluator{results: []*ai.PositionAnalysis{eval("a1", 0), eval("e5", 10), eval("b2", 0)}}
}

func TestClaimAnalysisJob_returnsOwnAttempt(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now()
	key := cacheKey(8, "advanced", "balanced", 2e9, "v2:moves=40")
	first, _, err := claimAnalysisJob(db, key, time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	mine, start, err := claimAnalysisJob(db, key, time.Minute, now.Add(time.Hour))
	if err != nil || !start {
		t.Fatalf("reclaim: start=%v err=%v", start, err)
	}
	if err := db.Model(&AnalysisJob{}).Where("id = ?", first.ID).Update("attempt", mine.Attempt+5).Error; err != nil {
		t.Fatal(err)
	}
	if mine.Attempt != first.Attempt+1 {
		t.Errorf("attempt = %d, want the one this caller wrote (%d)", mine.Attempt, first.Attempt+1)
	}
}

func TestRunAnalysisJob_supersededDoesNotPublish(t *testing.T) {
	g := playGame(t, []scriptedMove{{gotak.PlayerWhite, "a1"}, {gotak.PlayerBlack, "e5"}})
	db := setupTestDB(t)
	l := zap.NewNop().Sugar()
	key := cacheKey(g.ID, "advanced", "balanced", 2e9, gameCacheVersion(g))
	old, _, err := claimAnalysisJob(db, key, time.Minute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := claimAnalysisJob(db, key, time.Minute, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	runAnalysisJob(context.Background(), db, l, old, key, evFn(okEval()), g, time.Minute)
	if _, ok := loadAnalysisCache(db, l, key); ok {
		t.Error("a superseded attempt published its result")
	}
}

func TestRunAnalysisJob_evaluatorInitFails(t *testing.T) {
	g := playGame(t, []scriptedMove{{gotak.PlayerWhite, "a1"}})
	db := setupTestDB(t)
	key := cacheKey(g.ID, "advanced", "balanced", 2e9, gameCacheVersion(g))
	job, _, err := claimAnalysisJob(db, key, time.Minute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	fail := func() (positionEvaluator, error) { return nil, errors.New("no engine for this size") }
	runAnalysisJob(context.Background(), db, zap.NewNop().Sugar(), job, key, fail, g, time.Minute)
	var got AnalysisJob
	if err := db.First(&got, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != JobFailed || got.Error == "" {
		t.Errorf("job = %+v, want failed", got)
	}
}

func TestJobResponse_expiredRunningIsFailed(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now()
	job := &AnalysisJob{ID: 4, Status: JobRunning, ExpiresAt: now.Add(-time.Second)}
	resp := jobResponse(db, zap.NewNop().Sugar(), job, "alice-vs-bob", 5, now)
	if resp.Status != JobFailed || resp.Error == "" {
		t.Errorf("resp = %+v, want an abandoned job reported as failed", resp)
	}
	live := &AnalysisJob{ID: 5, Status: JobRunning, ExpiresAt: now.Add(time.Minute)}
	if resp := jobResponse(db, zap.NewNop().Sugar(), live, "alice-vs-bob", 5, now); resp.Status != JobRunning {
		t.Errorf("live job status = %q, want running", resp.Status)
	}
}
