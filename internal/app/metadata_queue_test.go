package app

import (
	"context"
	"testing"
)

func TestMetadataQueueAcceptsIndependentChangesAndRunsInOrder(t *testing.T) {
	a, _, songs := metadataLibraryFixture(t)
	var actor string
	a.DB.QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&actor)
	user := User{ID: actor}
	change := func(song Song, title string) metadataChange {
		return metadataChange{Song: song, Edit: MetadataEdit{Title: title, Artist: song.Artist, Album: song.Album, AlbumArtist: song.AlbumArtist}}
	}
	first, err := a.queueMetadata([]metadataChange{change(songs["one"], "First edit")}, user)
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.queueMetadata([]metadataChange{change(songs["two"], "Second edit")}, user)
	if err != nil {
		t.Fatalf("Independent request rejected: %v", err)
	}
	if _, err := a.queueMetadata([]metadataChange{change(songs["one"], "Conflicting edit")}, user); err == nil {
		t.Fatal("Overlapping snapshot accepted")
	}
	a.runMetadata(context.Background())
	state := func(id string) string {
		var value string
		a.DB.QueryRow("SELECT state FROM metadata_jobs WHERE id=?", id).Scan(&value)
		return value
	}
	if state(first) != "completed" || state(second) != "queued" {
		t.Fatalf("Wrong processing order: %s / %s", state(first), state(second))
	}
	a.runMetadata(context.Background())
	if state(second) != "completed" {
		t.Fatal("Second queued edit did not complete")
	}
}

func TestMetadataQueueProtectsMergeDestinationsAndAllowsIndependentRetry(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	var actor string
	a.DB.QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&actor)
	user := User{ID: actor}
	first := metadataChange{Song: songs["one"], Edit: MetadataEdit{Title: songs["one"].Title}}
	id, err := a.queueMetadata([]metadataChange{first}, user)
	if err != nil {
		t.Fatal(err)
	}
	// Reading a file another queued operation will change is unsafe.
	second := metadataChange{Song: songs["two"], Target: &first.Song}
	if _, err := a.queueMetadata([]metadataChange{second}, user); err == nil {
		t.Fatal("Changing merge destination accepted")
	}
	a.DB.Exec("UPDATE metadata_jobs SET state='failed' WHERE id=?", id)
	second.Target = nil
	if _, err := a.queueMetadata([]metadataChange{second}, user); err != nil {
		t.Fatal(err)
	}
	expect(t, admin.call("POST", "/api/v2/metadata/jobs", mustJSON(map[string]string{"id": id}), ""), 200)
	// A merge destination is also protected against a later write.
	a.DB.Exec("UPDATE metadata_jobs SET state='completed' WHERE state='queued'")
	second.Target = &first.Song
	if _, err := a.queueMetadata([]metadataChange{second}, user); err != nil {
		t.Fatal(err)
	}
	third := second
	third.Song.ID = "independent-source"
	if _, err := a.queueMetadata([]metadataChange{third}, user); err != nil {
		t.Fatalf("Shared read-only destination rejected: %v", err)
	}
	if _, err := a.queueMetadata([]metadataChange{first}, user); err == nil {
		t.Fatal("Write to queued merge destination accepted")
	}
}
