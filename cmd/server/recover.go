package main

import (
	"errors"
	"net/http"
	"runtime/debug"

	"go.icco.me/gutil/logging"
	"go.uber.org/zap"
)

// recoverPanics turns a handler panic into a logged 500. It sits inside
// the logging middleware so the request logger and trace are available.
func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := logging.FromContext(r.Context())
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			// net/http uses ErrAbortHandler to drop a connection on purpose.
			if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(rec)
			}
			l.Errorw("handler panicked", "panic", rec, "stack", string(debug.Stack()))
			if err := Renderer.JSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"}); err != nil {
				l.Errorw("failed to render JSON", zap.Error(err))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
