package app

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
)

func TestCurrentArtworkUsesReturnedIdentityAndRejectsNonImages(t *testing.T) {
	for _, mode := range []string{"song", "album", "missing", "json-error"} {
		t.Run(mode, func(t *testing.T) {
			a, err := Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			defer a.DB.Close()
			if err := a.createAdmin("admin", "test-password-long"); err != nil {
				t.Fatal(err)
			}
			admin := login(t, a, "admin")
			var picture bytes.Buffer
			if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.ParseForm()
				switch r.URL.Path {
				case "/rest/getSong.view":
					cover := "mf-specific-cover"
					if mode == "album" || mode == "missing" {
						cover = ""
					}
					send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "song": Song{ID: "one", CoverArt: cover, AlbumID: "album-one"}}})
				case "/rest/getAlbum.view":
					cover := "al-album-one"
					if mode == "missing" {
						cover = ""
					}
					send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "album": navAlbum{ID: "album-one", CoverArt: cover}}})
				case "/rest/getCoverArt.view":
					expected := "mf-specific-cover"
					if mode == "album" {
						expected = "al-album-one"
					}
					if r.Form.Get("id") != expected || r.Form.Get("size") != "300" {
						t.Errorf("Artwork request: %v", r.Form)
					}
					if mode == "json-error" {
						send(w, 200, map[string]string{"error": "Artwork missing"})
						return
					}
					w.Header().Set("Content-Type", "image/png")
					w.Write(picture.Bytes())
				default:
					t.Errorf("Unexpected endpoint: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			if err := a.seal("navidrome", Credentials{URL: server.URL}); err != nil {
				t.Fatal(err)
			}
			result := admin.call("GET", "/api/v2/metadata/songs/one/cover", "", "")
			switch mode {
			case "missing":
				expect(t, result, 404)
			case "json-error":
				expect(t, result, 502)
			default:
				expect(t, result, 200)
				if !bytes.Equal(result.Body.Bytes(), picture.Bytes()) || result.Header().Get("Content-Type") != "image/png" {
					t.Fatal("Image was not correctly proxied")
				}
			}
		})
	}
}
func TestSuggestionsCoverIdentityFieldsAndIndividualArtists(t *testing.T) {
	_, admin, songs := metadataLibraryFixture(t)
	s := songs["one"]
	s.Title = "Skate"
	s.Artist = "Vacio, ЛСП • Васильев Николай Михайлович, Олег Вадимович Савченко"
	s.Artists = []ArtistCredit{{ID: "vacio", Name: "Vacio"}, {ID: "lsp", Name: "ЛСП"}}
	s.AlbumArtist = "Vacio / ЛСП"
	s.AlbumArtists = s.Artists
	songs["one"] = s
	for _, field := range []string{"title", "artist", "album", "albumArtist"} {
		result := admin.call("GET", "/api/v2/metadata/suggestions?field="+field, "", "")
		expect(t, result, 200)
		var data struct{ Values []string }
		if err := json.Unmarshal(result.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		expected := map[string]string{"title": "Skate", "artist": "Vacio", "album": "Album", "albumArtist": "Vacio"}[field]
		found := false
		for _, value := range data.Values {
			if value == expected {
				found = true
			}
		}
		if !found {
			t.Fatal("Missing suggestion", field, expected, data.Values)
		}
	}
	matches := admin.call("GET", "/api/v2/metadata/suggestions?field=artist&q="+url.QueryEscape(s.Artist), "", "")
	expect(t, matches, 200)
	var suggestions struct{ Values []string }
	if err := json.Unmarshal(matches.Body.Bytes(), &suggestions); err != nil {
		t.Fatal(err)
	}
	if len(suggestions.Values) != 2 {
		t.Fatalf("Expected individual credits from full display query: %+v", suggestions.Values)
	}
	result := admin.call("GET", "/api/v2/metadata/songs/one", "", "")
	expect(t, result, 200)
	if !bytes.Contains(result.Body.Bytes(), []byte(`"searchArtist":"Vacio"`)) {
		t.Fatal("Artwork search should use the individual artist", result.Body.String())
	}
}
