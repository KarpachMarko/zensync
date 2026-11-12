package app

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func captureDiagnosticLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &logs
}
func TestAPICompletionLogsEveryStatusAndImplicitSuccess(t *testing.T) {
	logs := captureDiagnosticLogs(t, slog.LevelInfo)
	for _, status := range []int{0, 200, 201, 204, 401, 403, 404, 500} {
		logs.Reset()
		handler := apiDiagnostics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if status >= 400 {
				fail(w, status, "Diagnostic reason")
				return
			}
			if status != 0 {
				w.WriteHeader(status)
			}
		}))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "http://localhost/api/v2/example?token=private-query", nil))
		var record struct {
			Level  string `json:"level"`
			Status int    `json:"status"`
			ID     string `json:"request_id"`
			Bytes  int    `json:"response_bytes"`
		}
		if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
			t.Fatal(err, logs.String())
		}
		want := status
		if want == 0 {
			want = 200
		}
		if record.Status != want || record.ID == "" || record.ID != w.Header().Get("X-Request-Id") || record.Bytes != w.Body.Len() {
			t.Fatal(record, w.Body.String())
		}
		if strings.Contains(logs.String(), "private-query") {
			t.Fatal("query logged")
		}
		wantLevel := "INFO"
		if status >= 500 {
			wantLevel = "ERROR"
		} else if status >= 400 {
			wantLevel = "WARN"
		}
		if record.Level != wantLevel {
			t.Fatal(record)
		}
	}
}
func TestSuccessfulAPIAccessLogIncludesOwnerAndKeyWithoutCredentials(t *testing.T) {
	f := newDeletionFixture(t)
	c := soulKey(t, f.member, `["search:read"]`)
	logs := captureDiagnosticLogs(t, slog.LevelInfo)
	w := c.call("GET", "/api/v1/system/status", "")
	expect(t, w, 200)
	var record map[string]json.RawMessage
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatal(err, logs.String())
	}
	for _, field := range []string{"request_id", "user_id", "api_key_id", "auth", "method", "path", "status", "response_bytes", "duration_ms"} {
		if len(record[field]) == 0 {
			t.Fatalf("missing %s in %s", field, logs.String())
		}
	}
	if !strings.Contains(logs.String(), f.userID(t, f.member)) || strings.Contains(logs.String(), c.token) {
		t.Fatal(logs.String())
	}
}
func TestAPIDiagnosticsLogPanicsAndPreserveRecovery(t *testing.T) {
	logs := captureDiagnosticLogs(t, slog.LevelInfo)
	defer func() {
		if recover() != "private-panic" {
			t.Fatal("panic was swallowed")
		}
		if !strings.Contains(logs.String(), `"status":500`) || strings.Contains(logs.String(), "private-panic") {
			t.Fatal(logs.String())
		}
	}()
	apiDiagnostics(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("private-panic") })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "http://localhost/api/v2/example", nil))
}
func TestIntegrationDiagnosticsCorrelateAndRedactTransportURLs(t *testing.T) {
	logs := captureDiagnosticLogs(t, slog.LevelDebug)
	handler := apiDiagnostics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setRequestActor(r, "owner-id", "key-id")
		logIntegrationRequest(r.Context(), "deezer", "GET", "/search/track", time.Now(), nil, &url.Error{Op: "Get", URL: "https://private:password@example.com?token=secret", Err: &net.DNSError{Err: "private-dns-detail"}})
		w.WriteHeader(200)
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "http://localhost/api/v2/search?q=private-search", nil))
	output := logs.String()
	for _, secret := range []string{"password", "secret", "private-dns-detail", "private-search"} {
		if strings.Contains(output, secret) {
			t.Fatal("secret logged", output)
		}
	}
	for _, expected := range []string{`"service":"deezer"`, `"failure":"dns"`, `"user_id":"owner-id"`, `"api_key_id":"key-id"`} {
		if !strings.Contains(output, expected) {
			t.Fatal(output)
		}
	}
	if strings.Count(output, w.Header().Get("X-Request-Id")) != 3 {
		t.Fatal("request events are not correlated", output)
	}
}
func TestTransportFailureClassification(t *testing.T) {
	for _, entry := range []struct {
		err  error
		want string
	}{
		{context.Canceled, "cancelled"}, {context.DeadlineExceeded, "timeout"},
		{&net.DNSError{}, "dns"}, {&net.OpError{Op: "dial", Err: errors.New("private")}, "connection"},
		{x509.UnknownAuthorityError{}, "tls_certificate"}, {errors.New("private-transport-error"), "transport"},
	} {
		if got := transportFailure(&url.Error{Op: "Get", URL: "https://private.invalid?token=private", Err: entry.err}); got != entry.want {
			t.Fatal(got, entry.want)
		}
	}
}
func TestHealthProbeLoggingRequiresDebug(t *testing.T) {
	logs := captureDiagnosticLogs(t, slog.LevelInfo)
	handler := apiDiagnostics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "http://localhost/readyz", nil))
	if logs.Len() != 0 {
		t.Fatal(logs.String())
	}
}

func TestCancelledReadDoesNotReportConnectionFailure(t *testing.T) {
	logs := captureDiagnosticLogs(t, slog.LevelInfo)
	a := &App{HTTP: &http.Client{Transport: metadataTransport(func(r *http.Request) (*http.Response, error) { return nil, context.Canceled })}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.navRaw(ctx, Credentials{URL: "http://localhost", Username: "test", Password: "private"}, "search3", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Cancellation was hidden: %v", err)
	}
	handler := apiDiagnostics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fail(w, 502, "context canceled") }))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "http://localhost/api/v2/metadata/duplicates", nil).WithContext(ctx))
	var record struct {
		Level, Msg string
		Cancelled  bool
	}
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatal(err, logs.String())
	}
	if record.Level != "INFO" || record.Msg != "API request cancelled" || !record.Cancelled {
		t.Fatal(logs.String())
	}
}
