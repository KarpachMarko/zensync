package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Never log the raw transport error: url.Error includes the URL, which may
// carry provider tokens, passwords, signed download links or search terms.
func transportFailure(err error) string {
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "timeout"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "dns"
	}
	var certificate *tls.CertificateVerificationError
	if errors.As(err, &certificate) {
		return "tls_certificate"
	}
	var authority x509.UnknownAuthorityError
	if errors.As(err, &authority) {
		return "tls_certificate"
	}
	var operation *net.OpError
	if errors.As(err, &operation) {
		return "connection"
	}
	return "transport"
}
func logIntegrationRequest(ctx context.Context, service, method, endpoint string, started time.Time, resp *http.Response, err error) {
	logger := requestLogger(ctx)
	level := slog.LevelDebug
	status := 0
	failure := ""
	if resp != nil {
		status = resp.StatusCode
	}
	if err != nil {
		level = slog.LevelWarn
		failure = transportFailure(err)
	} else if status >= 300 {
		level = slog.LevelWarn
		failure = "http_status"
	}
	logger.Log(ctx, level, "Integration HTTP request", "service", service, "method", method, "endpoint", diagnosticPath(endpoint), "status", status, "failure", failure, "duration_ms", time.Since(started).Milliseconds())
}
