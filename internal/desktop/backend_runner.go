package desktop

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"quantix-connector-go/internal/api"
	"quantix-connector-go/internal/config"
	"quantix-connector-go/internal/service"
	"quantix-connector-go/internal/store"

	"gorm.io/gorm"
)

type BackendRunner struct {
	mu          sync.Mutex
	cfg         config.Settings
	db          *gorm.DB
	manager     *service.DeviceManager
	serialDebug *service.SerialDebugService
	httpServer  *http.Server
	ln          net.Listener
}

func NewBackendRunner(apiKey string) *BackendRunner {
	cfg := config.Load()
	cfg.BackendHost = "127.0.0.1"
	cfg.BackendPort = 8000
	cfg.APIKey = apiKey
	return &BackendRunner{cfg: cfg}
}

func (r *BackendRunner) Address() string {
	return fmt.Sprintf("http://%s:%d", r.cfg.BackendHost, r.cfg.BackendPort)
}

func (r *BackendRunner) APIKey() string {
	return r.cfg.APIKey
}

func (r *BackendRunner) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.httpServer != nil {
		return nil
	}
	addr := fmt.Sprintf("%s:%d", r.cfg.BackendHost, r.cfg.BackendPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("backend listen failed on %s: %w", addr, err)
	}

	db, err := store.OpenDB(r.cfg)
	if err != nil {
		_ = ln.Close()
		return err
	}
	manager := service.NewDeviceManager(db, r.cfg)
	serialDebug := service.NewSerialDebugService()
	if err := manager.Startup(context.Background()); err != nil {
		_ = ln.Close()
		return err
	}
	server := api.NewServer(r.cfg, db, manager, serialDebug)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	r.db = db
	r.manager = manager
	r.serialDebug = serialDebug
	r.httpServer = httpServer
	r.ln = ln

	go func() {
		if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("desktop backend http error: %v", err)
		}
	}()
	log.Printf("desktop backend started: %s", r.Address())
	return nil
}

func (r *BackendRunner) Stop(ctx context.Context) error {
	r.mu.Lock()
	httpServer := r.httpServer
	manager := r.manager
	ln := r.ln
	r.httpServer = nil
	r.manager = nil
	r.serialDebug = nil
	r.db = nil
	r.ln = nil
	r.mu.Unlock()

	var errs []error
	if httpServer != nil {
		if err := httpServer.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if ln != nil {
		_ = ln.Close()
	}
	if manager != nil {
		if err := manager.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("stop backend failed: %v", errs)
	}
	return nil
}

func (r *BackendRunner) Restart(apiKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	if err := r.Stop(ctx); err != nil {
		log.Printf("backend stop warning: %v", err)
	}
	r.mu.Lock()
	r.cfg.APIKey = apiKey
	r.mu.Unlock()
	return r.Start()
}
