package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"quantix-connector-go/internal/api"
	"quantix-connector-go/internal/config"
	"quantix-connector-go/internal/service"
	"quantix-connector-go/internal/store"
	"quantix-connector-go/internal/trayapp"
)

func main() {
	logPath := setupLogger()
	cfg := config.Load()
	db, err := store.OpenDB(cfg)
	if err != nil {
		log.Fatalf("open db failed: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("open sql db handle failed: %v", err)
	}
	manager := service.NewDeviceManager(db, cfg)
	serialDebug := service.NewSerialDebugService()
	printAgent := service.NewPrintAgentService(cfg.PrintAgent)
	if err := manager.Startup(context.Background()); err != nil {
		log.Fatalf("device manager startup failed: %v", err)
	}
	printAgent.Start(context.Background())

	server := api.NewServer(cfg, db, manager, serialDebug, printAgent)
	router := server.Router()

	httpServer := &http.Server{
		Addr:              cfg.BackendHost + ":" + itoa(cfg.BackendPort),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	httpErrCh := make(chan error, 1)
	go func() {
		log.Printf("Quantix server listening on http://%s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			select {
			case httpErrCh <- err:
			default:
			}
		}
	}()

	var shutdownOnce sync.Once
	shutdownDone := make(chan struct{})
	shutdown := func(reason string) {
		shutdownOnce.Do(func() {
			log.Printf("shutdown: %s", reason)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := httpServer.Shutdown(ctx); err != nil {
				log.Printf("http server shutdown error: %v", err)
			}
			if err := manager.Shutdown(ctx); err != nil {
				log.Printf("device manager shutdown error: %v", err)
			}
			if err := printAgent.Shutdown(ctx); err != nil {
				log.Printf("print agent shutdown error: %v", err)
			}
			if err := sqlDB.Close(); err != nil {
				log.Printf("db close error: %v", err)
			}
			close(shutdownDone)
		})
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		shutdown("signal: " + sig.String())
		trayapp.RequestQuit()
	}()
	go func() {
		err := <-httpErrCh
		log.Printf("http server error: %v", err)
		shutdown("http server error")
		trayapp.RequestQuit()
	}()

	frontendURL := fmt.Sprintf("http://%s", httpServer.Addr)
	err = trayapp.Run(trayapp.Options{
		FrontendURL: frontendURL,
		LogPath:     logPath,
		GetAPIKey:   server.CurrentAPIKey,
		UpdateAPIKey: func(v string) error {
			server.SetAPIKey(v)
			return config.SaveAPIKey(v)
		},
		OnQuit: func() {
			shutdown("tray menu")
		},
	})
	if err != nil {
		log.Printf("tray unavailable: %v", err)
		log.Println("fallback to signal mode; press Ctrl+C to exit")
		<-shutdownDone
	}
}

func itoa(v int) string {
	return fmt.Sprintf("%d", v)
}

func setupLogger() string {
	dir := filepath.Join(".", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("create logs dir failed: %v", err)
		return ""
	}
	path := filepath.Join(dir, "quantix.log")
	absPath, _ := filepath.Abs(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("open log file failed: %v", err)
		return absPath
	}
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("log file: %s", strings.TrimSpace(absPath))
	return absPath
}
