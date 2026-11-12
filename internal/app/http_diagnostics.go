package app

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type apiResponse struct {
	http.ResponseWriter
	status        int
	reason        string
	compatibility bool
	bytes         int64
	request       *http.Request
	writeFailed   bool
}

func (w *apiResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *apiResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *apiResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += int64(n)
	w.writeFailed = w.writeFailed || err != nil
	return n, err
}
func (w *apiResponse) apiError(status int, message string) {
	w.reason = message
	if w.compatibility {
		code := "INTERNAL_ERROR"
		switch status {
		case 400, 405, 413:
			code = "BAD_REQUEST"
		case 401:
			code = "INVALID_KEY"
		case 403:
			code = "NOT_AVAILABLE"
		case 404:
			code = "NOT_FOUND"
		case 409:
			code = "CONFLICT"
		case 429:
			code = "RATE_LIMITED"
		case 501, 502, 503:
			code = "NOT_AVAILABLE"
		}
		send(w, status, map[string]any{"success": false, "data": nil, "error": map[string]string{"code": code, "message": message}, "pagination": nil})
	} else {
		send(w, status, map[string]string{"error": message})
	}
}

// Request metadata contains identifiers only; never credentials or payloads.
type requestMetadata struct{ id, actor, key, auth string }
type requestMetadataKey struct{}
type diagnosticLoggerKey struct{}

func requestLogger(ctx context.Context) *slog.Logger {
	logger := slog.Default()
	if contextual, ok := ctx.Value(diagnosticLoggerKey{}).(*slog.Logger); ok {
		logger = contextual
	}
	if meta, ok := ctx.Value(requestMetadataKey{}).(*requestMetadata); ok {
		logger = logger.With("request_id", meta.id, "auth", meta.auth)
		if meta.actor != "" {
			logger = logger.With("user_id", meta.actor)
		}
		if meta.key != "" {
			logger = logger.With("api_key_id", meta.key)
		}
	}
	return logger
}
func setRequestActor(r *http.Request, actor, key string) {
	if meta, ok := r.Context().Value(requestMetadataKey{}).(*requestMetadata); ok {
		meta.actor = actor
		meta.key = key
		meta.auth = "session"
		if key != "" {
			meta.auth = "api_key"
		}
	}
}
func (w *apiResponse) logInternal(err error) {
	requestLogger(w.request.Context()).Error("database operation failed", "error", err)
}

// Completion logs cover successful and rejected API calls. Query strings,
// headers, cookies and request/response bodies are excluded. Health probes are
// debug-level to keep routine container checks out of the default access log.
func apiDiagnostics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		meta := &requestMetadata{id: id()[:16], auth: "anonymous"}
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			meta.auth = "api_key"
		} else if _, err := r.Cookie("zensync_session"); err == nil {
			meta.auth = "session"
		}
		r = r.WithContext(context.WithValue(r.Context(), requestMetadataKey{}, meta))
		response := &apiResponse{ResponseWriter: w, compatibility: strings.HasPrefix(r.URL.Path, "/api/v1/"), request: r}
		w.Header().Set("X-Request-Id", meta.id)
		path := diagnosticPath(r.URL.Path)
		requestLogger(r.Context()).Debug("API request started", "method", r.Method, "path", path)
		defer func() {
			panicValue := recover()
			if panicValue != nil {
				response.status = 500
				response.reason = "Unhandled handler panic"
			}
			if response.status == 0 {
				response.status = 200
			}
			level := slog.LevelInfo
			message := "API request completed"
			if response.status >= 500 {
				level = slog.LevelError
				message = "API request failed"
			} else if response.status >= 400 || response.writeFailed {
				level = slog.LevelWarn
				message = "API request failed"
			} else if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				level = slog.LevelDebug
			}
			if panicValue == nil && r.Context().Err() == context.Canceled {
				level = slog.LevelInfo
				message = "API request cancelled"
			}
			requestLogger(r.Context()).Log(r.Context(), level, message, "method", r.Method, "path", path, "status", response.status, "response_bytes", response.bytes, "reason", response.reason, "write_failed", response.writeFailed, "cancelled", r.Context().Err() != nil, "duration_ms", time.Since(started).Milliseconds())
			if panicValue != nil {
				panic(panicValue)
			}
		}()
		next.ServeHTTP(response, r)
	})
}
func diagnosticPath(path string) string {
	// IDs are unnecessary for diagnosing a route and may contain client secrets.
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if len(part) > 40 {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}
