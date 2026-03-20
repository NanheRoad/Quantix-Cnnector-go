package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"quantix-connector-go/internal/api"
	"quantix-connector-go/internal/config"
	"quantix-connector-go/internal/service"
	"quantix-connector-go/internal/store"
)

func main() {
	cfg := config.Load()
	db, err := store.OpenDB(cfg)
	if err != nil {
		log.Fatalf("open db failed: %v", err)
	}
	manager := service.NewDeviceManager(db, cfg)
	serialDebug := service.NewSerialDebugService()
	if err := manager.Startup(context.Background()); err != nil {
		log.Fatalf("device manager startup failed: %v", err)
	}

	server := api.NewServer(cfg, db, manager, serialDebug)
	router := server.Router()

	httpServer := &http.Server{
		Addr:              cfg.BackendHost + ":" + itoa(cfg.BackendPort),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("Quantix server listening on http://%s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("http server shutdown error: %v", err)
	}
	if err := manager.Shutdown(ctx); err != nil {
		log.Printf("device manager shutdown error: %v", err)
	}
}

func itoa(v int) string {
	return fmt.Sprintf("%d", v)
}
