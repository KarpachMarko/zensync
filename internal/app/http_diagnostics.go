package app

import (
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
	return w.ResponseWriter.Write(data)
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

// Query strings, credentials, cookies, request bodies are
// deliberately excluded. ResponseController can still reach the underlying writer.
func apiDiagnostics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		response := &apiResponse{ResponseWriter: w, compatibility: strings.HasPrefix(r.URL.Path, "/api/v1/")}
		requestID := id()[:16]
		w.Header().Set("X-Request-Id", requestID)
		next.ServeHTTP(response, r)
		if response.status >= 400 {
			slog.Warn("API request failed", "request_id", requestID, "method", r.Method, "path", diagnosticPath(r.URL.Path), "status", response.status, "reason", response.reason, "duration_ms", time.Since(started).Milliseconds())
		}
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
