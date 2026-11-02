package apple

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestCompleteSnapshot(t *testing.T) {
	data, e := os.ReadFile("testdata/library.xml")
	if e != nil {
		t.Fatal(e)
	}
	s, e := Parse(data)
	if e != nil {
		t.Fatal(e)
	}
	if len(s.Songs) != 4 || s.Excluded != 1 || s.Missing != 1 || s.Unplaylisted != 1 {
		t.Fatalf("bad counts: %+v", s)
	}
	if !s.Songs[0].Favorite || s.Songs[0].Original["Location"] != "file:///never/read/this" {
		t.Fatal("source metadata was lost")
	}
	p := s.Playlists[2]
	if strings.Join(p.Entries, ",") != "1,2,1,99" || p.ParentID != "FOLDER" {
		t.Fatalf("lost membership: %+v", p)
	}
}
func TestUntrustedXML(t *testing.T) {
	for _, input := range []string{`<!DOCTYPE plist [<!ENTITY x SYSTEM "file:///etc/passwd">]><plist><dict/></plist>`, `<plist><dict><key>Tracks</key><dict/><key>Tracks</key><dict/></dict></plist>`, `<plist><dict><key>Tracks</key><string><dict/></string></dict></plist>`, `<plist><dict><key>Tracks</key><dict/></dict></plist><extra/>`, `<plist>` + strings.Repeat(`<array>`, 40) + strings.Repeat(`</array>`, 40) + `</plist>`} {
		if _, e := Parse([]byte(input)); e == nil {
			t.Fatalf("accepted invalid input %s", input)
		}
	}
}

func BenchmarkLargeCatalog(b *testing.B) {
	var xml strings.Builder
	xml.WriteString(`<plist><dict><key>Tracks</key><dict>`)
	for i := 1; i <= 20000; i++ {
		fmt.Fprintf(&xml, `<key>%d</key><dict><key>Track ID</key><integer>%d</integer><key>Name</key><string>Song %d</string><key>Artist</key><string>Artist</string><key>Album</key><string>Album</string><key>Total Time</key><integer>180000</integer></dict>`, i, i, i)
	}
	xml.WriteString(`</dict><key>Playlists</key><array/></dict></plist>`)
	data := []byte(xml.String())
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, e := Parse(data)
		if e != nil || len(s.Songs) != 20000 {
			b.Fatalf("invalid large snapshot: %v", e)
		}
	}
}
