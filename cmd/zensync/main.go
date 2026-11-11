package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"zensync/internal/app"
)

func env(key, fallback string) string {
	if s := os.Getenv(key); s != "" {
		return s
	}
	return fallback
}
func main() {
	var level slog.Level
	levelErr := level.UnmarshalText([]byte(strings.ToUpper(env("ZENSYNC_LOG_LEVEL", "info"))))
	if levelErr != nil {
		level = slog.LevelInfo
	}
	logOutput := os.Stdout
	if len(os.Args) > 1 && os.Args[1] == "metadata-backups" {
		logOutput = os.Stderr
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(logOutput, &slog.HandlerOptions{Level: level})))
	if levelErr != nil {
		slog.Warn("Invalid ZENSYNC_LOG_LEVEL; using INFO")
	}
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		c := http.Client{Timeout: 3 * time.Second}
		r, e := c.Get("http://127.0.0.1:8080/readyz")
		if e != nil {
			os.Exit(1)
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	slog.Info("ZenSync starting", "log_level", level.String())
	data := env("ZENSYNC_DATA_DIR", "./data")
	if e := os.MkdirAll(data, 0700); e != nil {
		slog.Error("data directory unavailable", "error", e)
		os.Exit(1)
	}
	a, e := app.Open(filepath.Join(data, "zensync.db"))
	if e != nil {
		slog.Error("startup failed", "error", e)
		os.Exit(1)
	}
	slog.Info("Database and runtime initialized")
	defer a.DB.Close()
	if len(os.Args) > 1 && os.Args[1] == "metadata-backups" {
		if len(os.Args) != 3 || (os.Args[2] != "list" && os.Args[2] != "cleanup") {
			slog.Error("Usage: zensync metadata-backups list|cleanup")
			os.Exit(1)
		}
		unlock, err := a.AcquireWorkerLock()
		if err != nil {
			slog.Error("Stop ZenSync before running recovery maintenance", "error", err)
			os.Exit(1)
		}
		defer unlock()
		var entries []app.MetadataBackupInfo
		if os.Args[2] == "list" {
			entries, err = a.MetadataBackupInventory()
		} else {
			entries, err = a.CleanupMetadataBackups()
		}
		if entries != nil {
			if encodeErr := json.NewEncoder(os.Stdout).Encode(entries); encodeErr != nil {
				slog.Error("Could not write recovery inventory", "error", encodeErr)
				os.Exit(1)
			}
		}
		if err != nil {
			slog.Error("Metadata recovery maintenance failed", "error", err)
			os.Exit(1)
		}
		return
	}
	unlock, err := a.AcquireWorkerLock()
	if err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
	defer unlock()
	api := a.Handler()
	assets := env("ZENSYNC_WEB_DIR", "./web/dist")
	files := http.FileServer(http.Dir(assets))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			api.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self' data: https:; media-src 'self' https://*.dzcdn.net https://*.deezer.com; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			files.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		if r.URL.Path == "/icon.svg" {
			files.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(assets, "index.html"))
	})
	workerCtx, workerStop := context.WithCancel(context.Background())
	defer workerStop()
	workerDone := a.StartWorker(workerCtx)
	server := &http.Server{Addr: env("ZENSYNC_LISTEN", ":8080"), Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		slog.Info("Shutdown requested")
		workerStop()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(c); err != nil {
			slog.Error("HTTP shutdown failed", "error", err)
		}
	}()
	slog.Info("ZenSync listening", "address", server.Addr)
	if e = server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		slog.Error("server failed", "error", e)
		os.Exit(1)
	}
	workerStop()
	<-workerDone
	slog.Info("ZenSync stopped")
}
