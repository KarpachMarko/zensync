package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestMetadataScanTargetsChangedFoldersWithIncrementalFallback(t *testing.T) {
	a, _, _ := metadataLibraryFixture(t)
	var actor string
	a.DB.QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&actor)
	_, err := a.DB.Exec("INSERT INTO metadata_jobs(id,actor,changes,server_url) VALUES('scan-test',?,'[]','test')", actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		if _, err = a.DB.Exec("INSERT INTO metadata_files(job_id,song_id,file_path,before_hash,after_hash) VALUES('scan-test',?,?, 'before','after')", id, id+".flac"); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name                            string
		libraries                       int
		rejectTarget, missingPath, busy bool
		want                            []url.Values
	}{
		{name: "scanner busy", busy: true},
		{name: "targeted", libraries: 1, want: []url.Values{{"fullScan": {"false"}, "target": {"7:Aarne/SLAANG"}}}},
		{name: "unsupported targets", libraries: 1, rejectTarget: true, want: []url.Values{{"fullScan": {"false"}, "target": {"7:Aarne/SLAANG"}}, {"fullScan": {"false"}}}},
		{name: "multiple libraries", libraries: 2, want: []url.Values{{"fullScan": {"false"}}}},
		{name: "unknown path", libraries: 1, missingPath: true, want: []url.Values{{"fullScan": {"false"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			var scans []url.Values
			nav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.ParseForm()
				out := map[string]any{"status": "ok"}
				switch {
				case strings.Contains(r.URL.Path, "getScanStatus"):
					out["scanStatus"] = map[string]any{"scanning": test.busy}
				case strings.Contains(r.URL.Path, "getMusicFolders"):
					folders := []map[string]any{{"id": 7}}
					if test.libraries == 2 {
						folders = append(folders, map[string]any{"id": 8})
					}
					out["musicFolders"] = map[string]any{"musicFolder": folders}
				case strings.Contains(r.URL.Path, "getSong"):
					p := "Aarne/SLAANG/" + r.Form.Get("id") + ".flac"
					if test.missingPath {
						p = ""
					}
					out["song"] = map[string]any{"id": r.Form.Get("id"), "path": p}
				case strings.Contains(r.URL.Path, "startScan"):
					params := url.Values{"fullScan": {r.Form.Get("fullScan")}}
					if targets := r.Form["target"]; len(targets) > 0 {
						params["target"] = targets
					}
					mu.Lock()
					scans = append(scans, params)
					mu.Unlock()
					if test.rejectTarget && len(params["target"]) > 0 {
						out = map[string]any{"status": "failed", "error": map[string]any{"code": 0, "message": "unsupported target"}}
					}
				default:
					t.Error("Unexpected endpoint", r.URL.Path)
				}
				send(w, 200, map[string]any{"subsonic-response": out})
			}))
			defer nav.Close()
			if err := a.scanMetadataChanges(context.Background(), Credentials{URL: nav.URL, Username: "service", Password: "password"}, "scan-test"); err != nil && !(test.busy && errors.Is(err, errMetadataScanBusy)) {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if !reflect.DeepEqual(scans, test.want) {
				t.Fatalf("Scan parameters: got %v want %v", scans, test.want)
			}
		})
	}
}
