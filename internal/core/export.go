package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	Database string   `json:"database"`
	Roots    []string `json:"roots"`
	Enabled  bool     `json:"enabled"`
	DailyAt  string   `json:"dailyAt"`
}
type Record map[string]string
type Entry struct {
	Path   string `json:"path"`
	Title  string `json:"title"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Data   []byte `json:"-"`
}
type Report struct {
	Entries []Entry `json:"entries"`
	Written int     `json:"written"`
	Skipped int     `json:"skipped"`
	Errors  int     `json:"errors"`
}
type ID struct {
	Type    string `xml:"type,attr"`
	Default string `xml:"default,attr,omitempty"`
	Value   string `xml:",chardata"`
}
type Actor struct {
	Name  string `xml:"name"`
	Role  string `xml:"role,omitempty"`
	Order string `xml:"order,omitempty"`
}
type NFO struct {
	XMLName   xml.Name
	Title     string   `xml:"title"`
	Original  string   `xml:"originaltitle,omitempty"`
	Sort      string   `xml:"sorttitle,omitempty"`
	Plot      string   `xml:"plot,omitempty"`
	Year      string   `xml:"year,omitempty"`
	Premiered string   `xml:"premiered,omitempty"`
	Runtime   string   `xml:"runtime,omitempty"`
	Rating    string   `xml:"rating,omitempty"`
	Votes     string   `xml:"votes,omitempty"`
	Season    *int     `xml:"season,omitempty"`
	Episode   *int     `xml:"episode,omitempty"`
	ShowTitle string   `xml:"showtitle,omitempty"`
	IDs       []ID     `xml:"uniqueid"`
	Genres    []string `xml:"genre,omitempty"`
	Actors    []Actor  `xml:"actor,omitempty"`
	Directors []string `xml:"director,omitempty"`
}

type Querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func ReadRows(ctx context.Context, db Querier, query string) ([]Record, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var result []Record
	for rows.Next() {
		vs := make([]sql.NullString, len(names))
		ps := make([]any, len(names))
		for i := range vs {
			ps[i] = &vs[i]
		}
		if err = rows.Scan(ps...); err != nil {
			return nil, err
		}
		r := Record{}
		for i, n := range names {
			if vs[i].Valid {
				r[n] = vs[i].String
			}
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
func Validate(c Config) error {
	if !filepath.IsAbs(c.Database) || filepath.Base(c.Database) != "trimmedia.db" {
		return errors.New("数据库必须为 trimmedia.db 的绝对路径")
	}
	for _, r := range c.Roots {
		if !filepath.IsAbs(r) || filepath.Clean(r) == string(filepath.Separator) {
			return fmt.Errorf("不能使用此媒体根目录: %s", r)
		}
	}
	if _, err := strconv.Atoi(strings.ReplaceAll(c.DailyAt, ":", "")); err != nil {
		return errors.New("时间格式应为 HH:MM")
	}
	var h, m int
	if len(c.DailyAt) != 5 {
		return errors.New("时间格式应为 HH:MM")
	}
	if _, err := fmt.Sscanf(c.DailyAt, "%d:%d", &h, &m); err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return errors.New("时间格式应为 HH:MM")
	}
	return nil
}
func DSN(path string) string {
	uriPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	return (&url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)"}).String()
}
func Within(path, root string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func Allowed(path string, roots []string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	real, e := filepath.EvalSymlinks(path)
	if e != nil {
		return false
	}
	// Empty roots exports all database media, while still requiring existing absolute paths.
	if len(roots) == 0 {
		return true
	}
	for _, r := range roots {
		rr, e := filepath.EvalSymlinks(r)
		if e == nil && Within(real, rr) {
			return true
		}
	}
	return false
}
func MakeNFO(r Record, show string, genres []string, actors []Actor, directors []string) ([]byte, error) {
	kind := map[string]string{"Movie": "movie", "Episode": "episodedetails", "TV": "tvshow"}[r["type"]]
	if kind == "" {
		return nil, errors.New("不支持的媒体类型")
	}
	date := r["release_date"]
	if date == "" {
		date = r["first_air_date"]
	}
	if r["type"] == "Episode" {
		date = r["air_date"]
	}
	n := NFO{XMLName: xml.Name{Local: kind}, Title: r["title"], Original: r["original_title"], Sort: r["sort_title"], Plot: r["overview"], Premiered: date, Runtime: r["runtime"], Rating: r["vote_average"], Votes: r["vote_count"], Genres: genres, Actors: actors, Directors: directors}
	if len(date) >= 4 {
		n.Year = date[:4]
	}
	if r["type"] == "Episode" {
		s, e1 := strconv.Atoi(r["season_number"])
		ep, e2 := strconv.Atoi(r["episode_number"])
		if e1 != nil || e2 != nil {
			return nil, errors.New("缺少季集编号")
		}
		n.Season = &s
		n.Episode = &ep
		n.ShowTitle = show
	}
	for _, k := range []string{"tmdb", "imdb"} {
		v := r[k+"_id"]
		if r["type"] == "Episode" && k == "imdb" && r["episode_imdb_id"] != "" {
			v = r["episode_imdb_id"]
		}
		if v != "" && v != "0" {
			n.IDs = append(n.IDs, ID{Type: k, Value: v})
		}
	}
	if len(n.IDs) > 0 {
		n.IDs[0].Default = "true"
	}
	b, e := xml.MarshalIndent(n, "", "  ")
	return append(append([]byte(xml.Header), b...), '\n'), e
}

const managedMarker = "<!-- fnos-nfo-exporter:v1 sha256="

// The XML comment records ownership and a checksum of the exact generated body.
// Old/unmarked files are never adopted, even when their content matches.
func managedNFO(data []byte) []byte {
	return []byte(fmt.Sprintf("%s%s%x -->\n", data, managedMarker, sha256.Sum256(data)))
}

func unmodifiedNFO(data []byte) bool {
	i := bytes.LastIndex(data, []byte(managedMarker))
	return i >= 0 && bytes.Equal(data, managedNFO(data[:i]))
}

// Create without overwriting; replace only checksum-verified generated files.
func Publish(path string, data []byte, dry bool) (string, error) {
	if st, err := os.Lstat(path); err == nil && st.Mode()&os.ModeSymlink != 0 {
		return "symlink-skipped", nil
	}
	data = managedNFO(data)
	old, e := os.ReadFile(path)
	updating := e == nil
	if e == nil {
		if !unmodifiedNFO(old) {
			return "preserved", nil
		}
		if bytes.Equal(old, data) {
			return "unchanged", nil
		}
	}
	if e != nil && !os.IsNotExist(e) {
		return "", e
	}
	if dry {
		if updating {
			return "pending-update", nil
		}
		return "pending", nil
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".fnos-nfo-*.tmp")
	if e != nil {
		return "", e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, e = f.Write(data); e != nil {
		f.Close()
		return "", e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return "", e
	}
	if e = f.Chmod(0644); e != nil {
		f.Close()
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	if updating {
		// Recheck immediately before replacement to detect edits during generation.
		st, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return "symlink-skipped", nil
		}
		current, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if !bytes.Equal(current, old) {
			return "preserved", nil
		}
		if err = os.Rename(tmp, path); err != nil {
			return "", err
		}
		return "updated", nil
	}
	if e = os.Link(tmp, path); e != nil {
		if os.IsExist(e) {
			return "exists", nil
		}
		return "", e
	}
	return "written", nil
}
func Export(ctx context.Context, db *sql.DB, c Config, dry bool) (Report, error) {
	out := Report{Entries: []Entry{}}
	if e := Validate(c); e != nil {
		return out, e
	}
	tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	items, e := ReadRows(ctx, tx, "SELECT * FROM item")
	if e != nil {
		return out, e
	}
	byID := map[string]Record{}
	for _, r := range items {
		byID[r["guid"]] = r
	}
	media, e := ReadRows(ctx, tx, "SELECT item_guid,path FROM item_media ORDER BY path")
	if e != nil {
		return out, e
	}
	genres := map[string][]string{}
	tags, e := ReadRows(ctx, tx, "SELECT item_guid,tag FROM item_tag WHERE type='Genre' ORDER BY tag")
	if e != nil {
		return out, e
	}
	for _, r := range tags {
		genres[r["item_guid"]] = append(genres[r["item_guid"]], r["tag"])
	}
	actors := map[string][]Actor{}
	directors := map[string][]string{}
	people, e := ReadRows(ctx, tx, `SELECT ip.item_guid,p.name,ip.role,ip.job,ip.department,ip."order" FROM item_person ip JOIN person p ON p.guid=ip.person_guid ORDER BY ip.item_guid,ip."order",p.name`)
	if e != nil {
		return out, e
	}
	for _, p := range people {
		g := p["item_guid"]
		if p["department"] == "Acting" || p["job"] == "Actor" {
			actors[g] = append(actors[g], Actor{p["name"], p["role"], p["order"]})
		}
		if p["job"] == "Director" {
			directors[g] = append(directors[g], p["name"])
		}
	}
	if e = tx.Commit(); e != nil {
		return out, e
	}
	showFor := func(r Record) Record {
		for i := 0; i < 4 && r != nil; i++ {
			if r["type"] == "TV" {
				return r
			}
			r = byID[r["parent_guid"]]
		}
		return nil
	}
	targets := map[string]Entry{}
	conflicts := map[string]bool{}
	showDirs := map[string][]string{}
	add := func(r Record, path, show string) {
		data, err := MakeNFO(r, show, genres[r["guid"]], actors[r["guid"]], directors[r["guid"]])
		if err != nil {
			out.Errors++
			out.Entries = append(out.Entries, Entry{Path: path, Title: r["title"], Status: err.Error()})
			return
		}
		entry := Entry{Path: path, Title: r["title"], Type: r["type"], Data: data}
		if prev, ok := targets[path]; ok && !bytes.Equal(prev.Data, data) {
			conflicts[path] = true
		}
		targets[path] = entry
	}
	for _, m := range media {
		r := byID[m["item_guid"]]
		if r == nil || (r["type"] != "Movie" && r["type"] != "Episode") {
			continue
		}
		path := filepath.Clean(m["path"])
		if !Allowed(path, c.Roots) {
			out.Skipped++
			out.Entries = append(out.Entries, Entry{Path: path, Title: r["title"], Type: r["type"], Status: "missing-or-outside-roots"})
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			out.Skipped++
			continue
		}
		s := showFor(r)
		st := ""
		if s != nil {
			st = s["title"]
			showDirs[s["guid"]] = append(showDirs[s["guid"]], filepath.Dir(path))
		}
		add(r, strings.TrimSuffix(path, filepath.Ext(path))+".nfo", st)
	}
	for guid, dirs := range showDirs {
		root := dirs[0]
		for _, dir := range dirs[1:] {
			for !Within(dir, root) && filepath.Dir(root) != root {
				root = filepath.Dir(root)
			}
		}
		// A series with one season stored in Season 01 should get tvshow.nfo in its parent.
		base := strings.ToLower(filepath.Base(root))
		if strings.HasPrefix(base, "season ") || strings.HasPrefix(base, "season_") {
			root = filepath.Dir(root)
		}
		safe := filepath.Dir(root) != root && Allowed(root, c.Roots)
		for _, allowed := range c.Roots {
			if filepath.Clean(root) == filepath.Clean(allowed) {
				safe = false
			}
		}
		if safe {
			add(byID[guid], filepath.Join(root, "tvshow.nfo"), "")
		} else {
			out.Skipped++
			out.Entries = append(out.Entries, Entry{Title: byID[guid]["title"], Path: root, Type: "TV", Status: "ambiguous-show-root"})
		}
	}
	paths := make([]string, 0, len(targets))
	for p := range targets {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if e = ctx.Err(); e != nil {
			return out, e
		}
		entry := targets[p]
		if conflicts[p] {
			entry.Status = "conflicting-metadata"
			out.Errors++
		} else {
			// Check parent again; never follow a pre-existing output symlink.
			if !Allowed(filepath.Dir(p), c.Roots) {
				entry.Status = "outside-roots"
				out.Errors++
			} else if st, err := os.Lstat(p); err == nil && st.Mode()&os.ModeSymlink != 0 {
				entry.Status = "symlink-skipped"
				out.Skipped++
			} else {
				status, err := Publish(p, entry.Data, dry)
				entry.Status = status
				if err != nil {
					entry.Status = err.Error()
					out.Errors++
				} else if status == "written" || status == "updated" {
					out.Written++
				} else {
					out.Skipped++
				}
			}
		}
		out.Entries = append(out.Entries, entry)
	}
	return out, nil
}
func SaveJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, b, 0600)
}
