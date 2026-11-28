package main

import (
	"errors"
	"net/http"
	"runtime/debug"

	"github.com/go-chi/chi/v5/middleware"
	"go.icco.me/gutil/logging"
	"go.uber.org/zap"
)

// recoverPanics turns a handler panic into a logged 500. It sits inside
// routeTag so the failed request keeps its route label in metrics. When
// the handler already started the response, the status can no longer
// change, so the connection is aborted instead of sending a half-written
// success.
func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := logging.FromContext(r.Context())
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(rec)
			}
			l.Errorw("handler panicked", "panic", rec, "stack", string(debug.Stack()))
			if ww.Status() != 0 || ww.BytesWritten() > 0 {
				panic(http.ErrAbortHandler)
			}
			if err := Renderer.JSON(ww, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"}); err != nil {
				l.Errorw("failed to render JSON", zap.Error(err))
			}
		}()
		next.ServeHTTP(ww, r)
	})
}
