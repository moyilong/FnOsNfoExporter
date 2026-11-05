package core

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestMovieIsNotEpisodeWhenNumbersAreZero(t *testing.T) {
	b, e := MakeNFO(Record{"type": "Movie", "title": "冰河 & <世纪>", "season_number": "0", "episode_number": "0", "vote_average": "8.2", "vote_count": "109", "tmdb_id": "1"}, "", nil, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	var n NFO
	if e = xml.Unmarshal(b, &n); e != nil {
		t.Fatal(e)
	}
	if n.XMLName.Local != "movie" || n.Season != nil || n.Episode != nil || n.Rating != "8.2" || n.Votes != "109" {
		t.Fatalf("%+v", n)
	}
	if n.Title != "冰河 & <世纪>" {
		t.Fatal(n.Title)
	}
}
func TestSpecialEpisodeZero(t *testing.T) {
	b, e := MakeNFO(Record{"type": "Episode", "season_number": "0", "episode_number": "1", "title": "特辑"}, "剧名", nil, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(b, []byte("<season>0</season>")) {
		t.Fatal(string(b))
	}
}
func TestPublishIdempotentAndPreservesExisting(t *testing.T) {
	p := filepath.Join(t.TempDir(), "电影.nfo")
	s, e := Publish(p, []byte("first"), true)
	if e != nil || s != "pending" {
		t.Fatal(s, e)
	}
	if _, e = os.Stat(p); !os.IsNotExist(e) {
		t.Fatal("preview wrote file")
	}
	s, e = Publish(p, []byte("first"), false)
	if e != nil || s != "written" {
		t.Fatal(s, e)
	}
	old, _ := os.Stat(p)
	time.Sleep(time.Millisecond * 5)
	s, e = Publish(p, []byte("first"), false)
	if e != nil || s != "unchanged" {
		t.Fatal(s, e)
	}
	now, _ := os.Stat(p)
	if !old.ModTime().Equal(now.ModTime()) {
		t.Fatal("rewrote identical file")
	}
	s, e = Publish(p, []byte("new"), false)
	if e != nil || s != "exists" {
		t.Fatal(s, e)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "first" {
		t.Fatal("clobbered existing")
	}
}
func TestAllowedRootBoundary(t *testing.T) {
	d := t.TempDir()
	root := filepath.Join(d, "movies")
	other := filepath.Join(d, "movies-extra")
	os.Mkdir(root, 0755)
	os.Mkdir(other, 0755)
	p := filepath.Join(root, "a.mkv")
	os.WriteFile(p, nil, 0644)
	q := filepath.Join(other, "a.mkv")
	os.WriteFile(q, nil, 0644)
	if !Allowed(p, []string{root}) || Allowed(q, []string{root}) {
		rp, ep := filepath.EvalSymlinks(p)
		rr, er := filepath.EvalSymlinks(root)
		if runtime.GOOS == "windows" && (os.IsPermission(ep) || os.IsPermission(er)) {
			t.Skip("Windows sandbox denies EvalSymlinks; CI runs this check on Linux")
		}
		t.Fatalf("root boundary failed: p=%s root=%s pathError=%v rootError=%v", rp, rr, ep, er)
	}
	if Allowed(filepath.Join(root, "missing"), []string{root}) {
		t.Fatal("missing file allowed")
	}
}
func TestWithinLexicalBoundary(t *testing.T) {
	root := t.TempDir()
	if !Within(filepath.Join(root, "a.mkv"), root) || Within(root+"-extra", root) {
		t.Fatal("bad path boundary")
	}
}
func TestValidate(t *testing.T) {
	c := Config{Database: filepath.Join(t.TempDir(), "trimmedia.db"), Roots: []string{t.TempDir()}, DailyAt: "03:00"}
	if e := Validate(c); e != nil {
		t.Fatal(e)
	}
	c.DailyAt = "25:10"
	if Validate(c) == nil {
		t.Fatal("bad schedule accepted")
	}
}
