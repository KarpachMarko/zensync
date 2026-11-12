package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type interruptedAudioReader struct{ read bool }

func (r *interruptedAudioReader) Read(p []byte) (int, error) {
	if !r.read {
		r.read = true
		return copy(p, "a"), nil
	}
	return 0, io.ErrUnexpectedEOF
}
func TestOriginalAudioVerificationRetriesInterruptedReads(t *testing.T) {
	for _, mode := range []string{"interrupted", "truncated", "persistent", "different", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			a := &App{HTTP: &http.Client{Transport: metadataTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				var body io.Reader = strings.NewReader("abc")
				if mode == "different" {
					body = strings.NewReader("xyz")
				}
				if mode == "persistent" || (mode == "interrupted" && calls == 1) {
					body = &interruptedAudioReader{}
				}
				if mode == "truncated" && calls == 1 {
					body = strings.NewReader("a")
				}
				if mode == "cancelled" {
					return nil, context.Canceled
				}
				return &http.Response{StatusCode: 200, ContentLength: -1, Header: http.Header{}, Body: io.NopCloser(body)}, nil
			})}}
			checksum := sha256.Sum256([]byte("abc"))
			match, err := a.sameRemoteAudioHash(context.Background(), Credentials{URL: "http://localhost", Username: "test", Password: "private"}, "song", 3, hex.EncodeToString(checksum[:]))
			switch mode {
			case "interrupted", "truncated":
				if err != nil || !match || calls != 2 {
					t.Fatalf("%v %v %d", match, err, calls)
				}
			case "persistent":
				if err == nil || match || calls != 3 || !strings.Contains(err.Error(), "1/3 bytes") {
					t.Fatalf("%v %v %d", match, err, calls)
				}
			case "different":
				if err != nil || match || calls != 1 {
					t.Fatalf("Different file retried: %v %v %d", match, err, calls)
				}
			case "cancelled":
				if !errors.Is(err, context.Canceled) || calls != 1 {
					t.Fatalf("%v %d", err, calls)
				}
			}
		})
	}
}
