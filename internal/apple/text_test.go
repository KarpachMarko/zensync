package apple

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestAttachedTextPlaylist(t *testing.T) {
	data, err := os.ReadFile("testdata/playlist.txt")
	if err != nil {
		t.Fatal(err)
	}
	s, err := ParseFile(data, "test import.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Songs) != 13 || len(s.Playlists) != 1 || s.Unplaylisted != 0 || s.Excluded != 0 {
		t.Fatalf("bad snapshot: %+v", s)
	}
	p := s.Playlists[0]
	if p.Name != "test import" || len(p.Entries) != 13 || p.System || p.Folder {
		t.Fatalf("bad playlist: %+v", p)
	}
	for i, song := range s.Songs {
		if p.Entries[i] != song.SourceID || song.Favorite || song.Missing {
			t.Fatalf("bad membership: %+v", song)
		}
	}
	if s.Songs[0].Title != "Каждый день" || s.Songs[0].Duration != 99 || s.Songs[0].Artist != "LIL KRYSTALLL" || s.Songs[0].Album != "NO LABEL" || s.Songs[0].Original["Plays"] != "105" {
		t.Fatalf("lost metadata: %+v", s.Songs[0])
	}
	if s.Songs[6].Title != `Для Тебя (Пусто)` {
		t.Fatalf("lost Unicode: %+v", s.Songs[6])
	}
}

func TestTextPlaylistEncodingsAndOrder(t *testing.T) {
	for _, newline := range []string{"\r", "\n", "\r\n"} {
		text := strings.Join([]string{"Name\tArtist\tAlbum\tTime\tMy Rating", `"Song" 🎵` + "\tArtist\tAlbum\t123\t100", "Second\tArtist\tAlbum\t\t", `"Song" 🎵` + "\tArtist\tAlbum\t123\t100", ""}, newline)
		variants := [][]byte{[]byte(text), append([]byte{0xef, 0xbb, 0xbf}, []byte(text)...)}
		for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
			units := utf16.Encode([]rune("\ufeff" + text))
			data := make([]byte, len(units)*2)
			for i, u := range units {
				order.PutUint16(data[i*2:], u)
			}
			variants = append(variants, data)
		}
		for _, data := range variants {
			s, err := Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			if len(s.Songs) != 3 || len(s.Playlists[0].Entries) != 3 || s.Playlists[0].Name != "Imported playlist" {
				t.Fatalf("bad snapshot: %+v", s)
			}
			if s.Songs[0].Title != `"Song" 🎵` || s.Songs[2].Title != s.Songs[0].Title || s.Songs[0].Duration != 123 || s.Songs[0].Favorite {
				t.Fatalf("lost order or metadata: %+v", s.Songs)
			}
		}
	}
}

func TestInvalidTextPlaylist(t *testing.T) {
	for _, data := range [][]byte{
		nil, []byte("not a playlist"), {0xff, 0xfe, 0x01}, {0xff, 0xfe, 0x00, 0xd8}, {0xff},
		[]byte("Name\tArtist\tAlbum\tTime\tName"),
		[]byte("Name\tArtist\tAlbum\tTime\nSong\tArtist\tAlbum\tbad"),
		[]byte("Name\tArtist\tAlbum\tTime\nSong\tArtist\tAlbum\t-1"),
		[]byte("Name\tArtist\tAlbum\tTime\n\tArtist\tAlbum\t100"),
		[]byte("Name\tArtist\tAlbum\tTime\nSong\tArtist\tAlbum\t100\textra"),
		[]byte("Name\tArtist\tAlbum\tTime\nSong"),
		[]byte("Name\tArtist\tAlbum\tTime\n" + strings.Repeat("Song\tArtist\tAlbum\t100\n", 20001)),
		make([]byte, MaxBytes+1),
	} {
		if _, err := Parse(data); err == nil {
			t.Fatalf("accepted invalid export (length %d)", len(data))
		}
	}
}

func TestSinglePlaylistXML(t *testing.T) {
	s, err := ParseFile([]byte(`<plist><dict><key>Tracks</key><dict><key>1</key><dict><key>Name</key><string>Only song</string></dict></dict><key>Playlists</key><array><dict><key>Name</key><string>Single playlist</string><key>Playlist Items</key><array><dict><key>Track ID</key><integer>1</integer></dict><dict><key>Track ID</key><integer>1</integer></dict></array></dict></array></dict></plist>`), "ignored.xml")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Songs) != 1 || len(s.Playlists) != 1 || s.Playlists[0].Name != "Single playlist" || strings.Join(s.Playlists[0].Entries, ",") != "1,1" {
		t.Fatalf("bad playlist: %+v", s)
	}
}
