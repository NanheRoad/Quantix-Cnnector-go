package api

import (
	"context"
	"net/http"
	"time"

	"quantix-connector-go/internal/service"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerRemotePrintRoutes(rg *gin.RouterGroup) {
	r := rg.Group("/api/remote-print")
	r.POST("/jobs", s.remotePrintJob)
}

func (s *Server) remotePrintJob(c *gin.Context) {
	if s.printAgent == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "print agent unavailable"})
		return
	}
	var req service.DirectPrintJob
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	record, err := s.printAgent.ExecuteDirectJob(ctx, req)
	if err != nil && record.JobCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"ok":      false,
			"status":  record.Status,
			"message": record.Message,
			"job":     record,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"status":  record.Status,
		"message": record.Message,
		"job":     record,
	})
}
