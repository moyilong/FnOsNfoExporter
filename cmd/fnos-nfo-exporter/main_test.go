package main

import (
	"context"
	"database/sql"
	"encoding/xml"
	"github.com/moyil/fnos-nfo-exporter/internal/core"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDailySchedule(t *testing.T) {
	loc := time.FixedZone("Shanghai", 8*3600)
	now := time.Date(2026, 11, 5, 4, 0, 0, 0, loc)
	n := nextDaily(now, core.Config{DailyAt: "03:00"})
	if n.Day() != 6 || n.Hour() != 3 {
		t.Fatal(n)
	}
}
func TestSQLiteExportAndReadOnly(t *testing.T) {
	root := t.TempDir()
	database := filepath.Join(t.TempDir(), "trimmedia.db")
	db, e := sql.Open("sqlite", database)
	if e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{
		"CREATE TABLE item(guid TEXT,type TEXT,title TEXT,original_title TEXT,overview TEXT,release_date TEXT,first_air_date TEXT,air_date TEXT,runtime INTEGER,vote_average REAL,vote_count INTEGER,tmdb_id INTEGER,imdb_id TEXT,season_number INTEGER,episode_number INTEGER,parent_guid TEXT)",
		"CREATE TABLE item_media(item_guid TEXT,path TEXT)",
		"CREATE TABLE item_tag(item_guid TEXT,tag TEXT,type TEXT)",
		"CREATE TABLE person(guid TEXT,name TEXT)",
		`CREATE TABLE item_person(item_guid TEXT,person_guid TEXT,role TEXT,job TEXT,department TEXT,"order" INTEGER)`,
		"INSERT INTO item(guid,type,title,season_number,episode_number,vote_average,vote_count) VALUES('m1','Movie','第一部',0,0,8.5,123),('m2','Movie','第二部',0,0,7,222)",
	} {
		if _, e = db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	for i, id := range []string{"m1", "m2"} {
		p := filepath.Join(root, []string{"a.mkv", "b.mkv"}[i])
		os.WriteFile(p, []byte("video"), 0644)
		if _, e = db.Exec("INSERT INTO item_media VALUES(?,?)", id, p); e != nil {
			t.Fatal(e)
		}
	}
	db.Close()
	db, e = sql.Open("sqlite", core.DSN(database))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.Exec("DELETE FROM item"); e == nil {
		t.Fatal("database is writable")
	}
	c := core.Config{Database: database, Roots: []string{root}, DailyAt: "03:00"}
	rep, e := core.Export(context.Background(), db, c, true)
	if e != nil {
		t.Fatal(e)
	}
	if len(rep.Entries) != 2 || rep.Written != 0 {
		t.Fatalf("%+v", rep)
	}
	rep, e = core.Export(context.Background(), db, c, false)
	if e != nil || rep.Written != 2 {
		t.Fatal(rep, e)
	}
	for _, n := range []string{"a.nfo", "b.nfo"} {
		b, e := os.ReadFile(filepath.Join(root, n))
		if e != nil {
			t.Fatal(e)
		}
		var out core.NFO
		if e = xml.Unmarshal(b, &out); e != nil || out.XMLName.Local != "movie" {
			t.Fatal(out, e)
		}
	}
	rep, e = core.Export(context.Background(), db, c, false)
	if e != nil || rep.Written != 0 {
		t.Fatal(rep, e)
	}
}
