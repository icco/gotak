package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.icco.me/gotak"
	"go.icco.me/gutil/logging"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// syncAnalyzeBudget is the longest game walk answered inline. Past it the
// request starts a background job, since proxies and mobile clients time
// out long before a big game finishes.
const syncAnalyzeBudget = 20 * time.Second

// Analysis job states.
const (
	JobRunning = "running"
	JobDone    = "done"
	JobFailed  = "failed"
)

// AnalysisJob tracks a background game analysis. It shares its key with
// AnalysisCache, and the result lands in the cache when the job is done.
// ExpiresAt is when a running job counts as abandoned (its instance died)
// and may be claimed again.
type AnalysisJob struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	GameID      int64     `gorm:"not null;uniqueIndex:idx_analysis_job_key,priority:1" json:"game_id"`
	Level       string    `gorm:"type:varchar(16);not null;uniqueIndex:idx_analysis_job_key,priority:2" json:"level"`
	Style       string    `gorm:"type:varchar(16);not null;uniqueIndex:idx_analysis_job_key,priority:3" json:"style"`
	TimeLimitNs int64     `gorm:"not null;uniqueIndex:idx_analysis_job_key,priority:4" json:"time_limit_ns"`
	GameVersion string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_analysis_job_key,priority:5" json:"game_version"`
	Status      string    `gorm:"type:varchar(16);not null" json:"status"`
	Error       string    `gorm:"type:text" json:"error,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
	Attempt     int       `gorm:"not null;default:0" json:"attempt"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AnalyzeJobResponse reports a background analysis. Result is set once
// Status is done.
type AnalyzeJobResponse struct {
	ID        int64            `json:"id"`
	Status    string           `json:"status"`
	StatusURL string           `json:"status_url"`
	Error     string           `json:"error,omitempty"`
	Result    *AnalyzeResponse `json:"result,omitempty"`
}

func jobKey(k analysisCacheKey) AnalysisJob {
	return AnalysisJob{
		GameID:      k.gameID,
		Level:       k.level,
		Style:       k.style,
		TimeLimitNs: k.timeLimitNs,
		GameVersion: k.gameVersion,
	}
}

// claimAnalysisJob finds or creates the job for k. It reports start=true
// when the caller now owns the job and must run it: the job is new, or the
// last run failed, finished without a cached result, or was abandoned.
// The returned Attempt is the caller's fencing token; a conditional update
// makes concurrent claimers race safely.
func claimAnalysisJob(db *gorm.DB, k analysisCacheKey, budget time.Duration, now time.Time) (*AnalysisJob, bool, error) {
	expires := now.Add(budget + analyzeDeadlineSlack)
	row := jobKey(k)
	row.Status = JobRunning
	row.ExpiresAt = expires
	res := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if res.Error != nil {
		return nil, false, res.Error
	}
	if res.RowsAffected == 1 {
		return &row, true, nil
	}

	var job AnalysisJob
	q := jobKey(k)
	if err := db.Where(&q).First(&job).Error; err != nil {
		return nil, false, err
	}
	if job.Status == JobRunning && now.Before(job.ExpiresAt) {
		return &job, false, nil
	}

	next := job.Attempt + 1
	upd := db.Model(&AnalysisJob{}).
		Where("id = ? AND attempt = ?", job.ID, job.Attempt).
		Updates(map[string]any{"status": JobRunning, "error": "", "expires_at": expires, "attempt": next})
	if upd.Error != nil {
		return nil, false, upd.Error
	}
	if upd.RowsAffected == 1 {
		// Use what this caller wrote, not a reread, which could already
		// belong to a later claimer.
		job.Status, job.Error, job.ExpiresAt, job.Attempt = JobRunning, "", expires, next
		return &job, true, nil
	}
	if err := db.First(&job, job.ID).Error; err != nil {
		return nil, false, err
	}
	return &job, false, nil
}

// runAnalysisJob builds an evaluator, waits for an engine slot, walks the
// game, and publishes the result. ctx must outlive the request that
// started the job.
func runAnalysisJob(ctx context.Context, db *gorm.DB, l *zap.SugaredLogger, job *AnalysisJob, k analysisCacheKey, newEv func() (positionEvaluator, error), game *gotak.Game, budget time.Duration) {
	actx, cancel := context.WithTimeout(ctx, budget+analyzeDeadlineSlack/2)
	defer cancel()

	ev, err := newEv()
	if err != nil {
		finishAnalysisJob(db, l, job, k, nil, 0, fmt.Sprintf("could not start the engine: %v", err))
		return
	}
	release, err := acquireSlot(actx, engineSlots, budget)
	if err != nil {
		finishAnalysisJob(db, l, job, k, nil, 0, fmt.Sprintf("no engine slot: %v", err))
		return
	}
	moves := analyzeGame(actx, ev, game)
	release()

	agreed := 0
	failure := ""
	for i, m := range moves {
		if m.Agreed {
			agreed++
		}
		if m.Error != "" && failure == "" {
			failure = fmt.Sprintf("move %d: %s", i, m.Error)
		}
	}
	finishAnalysisJob(db, l, job, k, moves, agreed, failure)
}

// finishAnalysisJob records the outcome and, on success, the cached result
// in one transaction fenced on the attempt, so a superseded run can
// neither publish a result nor overwrite the current status.
func finishAnalysisJob(db *gorm.DB, l *zap.SugaredLogger, job *AnalysisJob, k analysisCacheKey, moves []MoveAnalysis, agreed int, failure string) {
	status := JobDone
	if failure != "" {
		status = JobFailed
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		upd := tx.Model(&AnalysisJob{}).Where("id = ? AND attempt = ?", job.ID, job.Attempt).
			Updates(map[string]any{"status": status, "error": failure})
		if upd.Error != nil {
			return upd.Error
		}
		if upd.RowsAffected == 0 {
			l.Infow("analysis job superseded, dropping result", "job", job.ID, "attempt", job.Attempt)
			return nil
		}
		if status == JobDone {
			saveAnalysisCache(tx, l, k, agreed, moves)
		}
		return nil
	})
	if err != nil {
		l.Errorw("could not record analysis job result", "job", job.ID, zap.Error(err))
		return
	}
	l.Infow("analysis job finished", "job", job.ID, "status", status, "error", failure)
}

func jobResponse(db *gorm.DB, l *zap.SugaredLogger, job *AnalysisJob, slug string, size int64, now time.Time) AnalyzeJobResponse {
	resp := AnalyzeJobResponse{
		ID:        job.ID,
		Status:    job.Status,
		StatusURL: fmt.Sprintf("/analyze/game/%s/jobs/%d", slug, job.ID),
		Error:     job.Error,
	}
	if job.Status == JobRunning && !now.Before(job.ExpiresAt) {
		resp.Status = JobFailed
		resp.Error = "analysis was abandoned; request it again to restart"
		return resp
	}
	if job.Status != JobDone {
		return resp
	}
	k := analysisCacheKey{
		gameID:      job.GameID,
		level:       job.Level,
		style:       job.Style,
		timeLimitNs: job.TimeLimitNs,
		gameVersion: job.GameVersion,
	}
	cached, ok := loadAnalysisCache(db, l, k)
	if !ok {
		resp.Status = JobFailed
		resp.Error = "result is no longer cached; request the analysis again"
		return resp
	}
	cached.Slug = slug
	cached.Size = size
	cached.Level = job.Level
	cached.CriticalMoments = criticalMoments(cached.Moves)
	resp.Result = &cached
	return resp
}

// @Summary Get a background game analysis
// @Description Reports a job started by POST /analyze/game/{slug} for a
// @Description long game: running, failed with an error, or done with the
// @Description full analysis in result.
// @Tags analysis
// @Produce json
// @Param slug path string true "Game slug identifier"
// @Param id path int true "Job id"
// @Success 200 {object} AnalyzeJobResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /analyze/game/{slug}/jobs/{id} [get]
func getAnalyzeJobHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := logging.FromContext(ctx)

	id, err := strconv.ParseInt(chi.URLParamFromCtx(ctx, "id"), 10, 64)
	if err != nil {
		if jerr := Renderer.JSON(w, http.StatusNotFound, ErrorResponse{Error: "job not found"}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	db, game, ok := loadGameForReadWithDB(w, r, l)
	if !ok {
		return
	}

	var job AnalysisJob
	if err := db.Where("id = ? AND game_id = ?", id, game.ID).First(&job).Error; err != nil {
		status, msg := http.StatusInternalServerError, "could not load job"
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status, msg = http.StatusNotFound, "job not found"
		} else {
			l.Errorw("could not load analysis job", "id", id, zap.Error(err))
		}
		if jerr := Renderer.JSON(w, status, ErrorResponse{Error: msg}); jerr != nil {
			l.Errorw("failed to render JSON", zap.Error(jerr))
		}
		return
	}

	if err := Renderer.JSON(w, http.StatusOK, jobResponse(db, l, &job, game.Slug, game.Board.Size, time.Now())); err != nil {
		l.Errorw("failed to render analysis job", zap.Error(err))
	}
}
