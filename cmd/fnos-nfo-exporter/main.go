package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/moyil/fnos-nfo-exporter/internal/core"
	"log"
	_ "modernc.org/sqlite"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed index.html
var page []byte

type State struct {
	Running bool        `json:"running"`
	Last    string      `json:"last"`
	Error   string      `json:"error"`
	Next    string      `json:"next"`
	Report  core.Report `json:"report"`
}
type App struct {
	mu       sync.Mutex
	cfg      core.Config
	state    State
	dir      string
	password string
	ctx      context.Context
}

func main() {
	dir := flag.String("data", env("TRIM_PKGVAR", "./data"), "应用数据目录")
	listen := flag.String("listen", ":18080", "HTTP 地址")
	init := flag.Bool("init", false, "从 wizard_password 初始化控制台密码")
	inspect := flag.String("inspect", "", "只读检查 trimmedia.db")
	flag.Parse()
	if *inspect != "" {
		db, e := sql.Open("sqlite", core.DSN(*inspect))
		if e != nil {
			log.Fatal(e)
		}
		defer db.Close()
		r, e := core.ReadRows(context.Background(), db, "SELECT type,count(*) AS count FROM item GROUP BY type")
		if e != nil {
			log.Fatal(e)
		}
		json.NewEncoder(os.Stdout).Encode(r)
		return
	}
	if e := os.MkdirAll(*dir, 0700); e != nil {
		log.Fatal(e)
	}
	passfile := filepath.Join(*dir, "password.sha256")
	if *init {
		p := os.Getenv("wizard_password")
		if len(p) < 12 {
			log.Fatal("控制台密码至少 12 个字符")
		}
		sum := sha256.Sum256([]byte(p))
		if e := os.WriteFile(passfile, []byte(hex.EncodeToString(sum[:])), 0600); e != nil {
			log.Fatal(e)
		}
		return
	}
	hash, e := os.ReadFile(passfile)
	if e != nil {
		log.Fatal("请通过安装向导设置控制台密码，或以 wizard_password 环境变量运行 --init")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a := &App{dir: *dir, password: strings.TrimSpace(string(hash)), ctx: ctx, cfg: core.Config{DailyAt: "03:00", Roots: []string{}}}
	if b, e := os.ReadFile(filepath.Join(*dir, "config.json")); e == nil {
		if e = json.Unmarshal(b, &a.cfg); e != nil {
			log.Fatal(e)
		}
	} else if !os.IsNotExist(e) {
		log.Fatal(e)
	} else {
		a.cfg.Database = detectDB()
	}
	if b, e := os.ReadFile(filepath.Join(*dir, "last-run.json")); e == nil {
		json.Unmarshal(b, &a.state)
		a.state.Running = false
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.index)
	mux.HandleFunc("/api/config", a.config)
	mux.HandleFunc("/api/run", a.runHTTP)
	mux.HandleFunc("/api/status", a.status)
	server := &http.Server{Addr: *listen, Handler: a.auth(mux), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go a.schedule()
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(c)
	}()
	log.Printf("控制台监听 %s", *listen)
	if e := server.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
		log.Fatal(e)
	}
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func detectDB() string {
	const defaultDB = "/usr/local/apps/@appdata/trim.media/database/trimmedia.db"
	var paths []string
	for _, pattern := range []string{defaultDB, "/var/apps/trim.media/meta/trimmedia.db", "/vol*/@appmeta/trim.media/trimmedia.db", "/vol*/@appdata/trim.media/trimmedia.db"} {
		p, _ := filepath.Glob(pattern)
		paths = append(paths, p...)
	}
	for _, p := range paths {
		if s, e := os.Stat(p); e == nil && s.Size() > 0 {
			return p
		}
	}
	return defaultDB
}
func (a *App) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		u, p, ok := r.BasicAuth()
		h := sha256.Sum256([]byte(p))
		if !ok || u != "admin" || subtle.ConstantTimeCompare([]byte(hex.EncodeToString(h[:])), []byte(a.password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="NFO Export", charset="UTF-8"`)
			http.Error(w, "请使用 admin 和安装时的密码登录", 401)
			return
		}
		if r.Method == "POST" {
			if r.Header.Get("X-NFO-Request") != "true" {
				http.Error(w, "missing request header", 403)
				return
			}
			if o := r.Header.Get("Origin"); o != "" {
				v, e := url.Parse(o)
				if e != nil || v.Host != r.Host {
					http.Error(w, "invalid origin", 403)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (a *App) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" || r.Method != "GET" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page)
}
func respond(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func (a *App) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	respond(w, a.state)
}
func (a *App) config(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.Method == "GET" {
		respond(w, a.cfg)
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	if a.state.Running {
		http.Error(w, "任务运行时不能更改设置", 409)
		return
	}
	var c core.Config
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	d.DisallowUnknownFields()
	if e := d.Decode(&c); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if e := core.Validate(c); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if e := core.SaveJSON(filepath.Join(a.dir, "config.json"), c); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	a.cfg = c
	a.state.Next = ""
	if c.Enabled {
		a.state.Next = nextDaily(time.Now(), c).Format(time.RFC3339)
	}
	respond(w, c)
}
func (a *App) runHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	dry := r.URL.Query().Get("preview") == "true"
	if e := a.start(dry); e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	w.WriteHeader(202)
	respond(w, map[string]bool{"accepted": true})
}
func (a *App) start(dry bool) error {
	a.mu.Lock()
	if a.state.Running {
		a.mu.Unlock()
		return errors.New("已有任务运行中")
	}
	c := a.cfg
	if e := core.Validate(c); e != nil {
		a.mu.Unlock()
		return e
	}
	a.state.Running = true
	a.state.Error = ""
	a.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 30*time.Minute)
		defer cancel()
		db, err := sql.Open("sqlite", core.DSN(c.Database))
		var result core.Report
		if err == nil {
			db.SetMaxOpenConns(1)
			result, err = core.Export(ctx, db, c, dry)
			db.Close()
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		a.state.Running = false
		a.state.Last = time.Now().Format(time.RFC3339)
		a.state.Report = result
		if err != nil {
			a.state.Error = err.Error()
		}
		if e := core.SaveJSON(filepath.Join(a.dir, "last-run.json"), a.state); e != nil {
			a.state.Error += "; 保存报告失败: " + e.Error()
		}
		log.Printf("任务结束 preview=%v written=%d errors=%d error=%v", dry, result.Written, result.Errors, err)
	}()
	return nil
}
func nextDaily(now time.Time, c core.Config) time.Time {
	var h, m int
	fmt.Sscanf(c.DailyAt, "%d:%d", &h, &m)
	t := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}
func (a *App) schedule() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	var due time.Time
	var previous string
	for {
		select {
		case <-a.ctx.Done():
			return
		case now := <-ticker.C:
			a.mu.Lock()
			c := a.cfg
			key := fmt.Sprintf("%v/%s", c.Enabled, c.DailyAt)
			if !c.Enabled {
				a.state.Next = ""
				due = time.Time{}
			} else {
				if key != previous || due.IsZero() {
					due = nextDaily(now, c)
				}
				a.state.Next = due.Format(time.RFC3339)
			}
			previous = key
			fire := c.Enabled && !due.After(now)
			if fire {
				due = nextDaily(now, c)
				a.state.Next = due.Format(time.RFC3339)
			}
			a.mu.Unlock()
			if fire {
				if e := a.start(false); e != nil {
					log.Printf("计划任务跳过: %v", e)
				}
			}
		}
	}
}
