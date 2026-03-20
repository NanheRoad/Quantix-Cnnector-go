package api

import (
	"io/fs"
	"net/http"
	"strings"

	webembed "quantix-connector-go"
	"quantix-connector-go/internal/config"
	"quantix-connector-go/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Server struct {
	cfg         config.Settings
	db          *gorm.DB
	manager     *service.DeviceManager
	serialDebug *service.SerialDebugService
	webFS       fs.FS
	staticFS    http.FileSystem
}

func NewServer(cfg config.Settings, db *gorm.DB, manager *service.DeviceManager, serialDebug *service.SerialDebugService) *Server {
	s := &Server{cfg: cfg, db: db, manager: manager, serialDebug: serialDebug}
	webFS, err := webembed.WebFS()
	if err == nil {
		s.webFS = webFS
		if staticFS, subErr := fs.Sub(webFS, "static"); subErr == nil {
			s.staticFS = http.FS(staticFS)
		}
	}
	return s
}

func (s *Server) Router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, s.manager.HealthSnapshot())
	})
	r.GET("/", s.serveIndex)
	if s.staticFS != nil {
		r.StaticFS("/static", s.staticFS)
	} else {
		r.Static("/static", "web/static")
	}
	r.GET("/openapi.json", s.openapi)
	r.GET("/docs", s.docsPage)
	r.GET("/ws", s.websocketStream)

	protected := r.Group("/")
	protected.Use(s.requireAPIKey())
	s.registerDeviceRoutes(protected)
	s.registerProtocolRoutes(protected)
	s.registerCategoryRoutes(protected)
	s.registerSerialDebugRoutes(protected)
	return r
}

func (s *Server) verifyAPIKey(v string) bool {
	if strings.TrimSpace(s.cfg.APIKey) == "" {
		return true
	}
	return strings.TrimSpace(v) == strings.TrimSpace(s.cfg.APIKey)
}

func (s *Server) requireAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("X-API-Key")
		query := c.Query("api_key")
		key := header
		if key == "" {
			key = query
		}
		if !s.verifyAPIKey(key) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "Invalid API key"})
			return
		}
		c.Next()
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "*")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
