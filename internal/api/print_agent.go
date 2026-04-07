package api

import (
	"context"
	"net/http"
	"time"

	"quantix-connector-go/internal/config"
	"quantix-connector-go/internal/service"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerPrintAgentRoutes(rg *gin.RouterGroup) {
	r := rg.Group("/api/print-agent")
	r.GET("/status", s.printAgentStatus)
	r.GET("/config", s.printAgentConfig)
	r.GET("/bartender-candidates", s.printAgentBarTenderCandidates)
	r.PUT("/config", s.updatePrintAgentConfig)
	r.GET("/jobs", s.printAgentJobs)
	r.POST("/poll-once", s.printAgentPollOnce)
}

func (s *Server) printAgentStatus(c *gin.Context) {
	if s.printAgent == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "print agent unavailable"})
		return
	}
	c.JSON(http.StatusOK, s.printAgent.Status())
}

func (s *Server) printAgentConfig(c *gin.Context) {
	if s.printAgent == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "print agent unavailable"})
		return
	}
	c.JSON(http.StatusOK, s.printAgent.CurrentConfig())
}

func (s *Server) printAgentBarTenderCandidates(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"items": service.ListBarTenderExecutableCandidates(),
	})
}

func (s *Server) updatePrintAgentConfig(c *gin.Context) {
	if s.printAgent == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "print agent unavailable"})
		return
	}
	var req config.PrintAgentSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if err := config.SavePrintAgentSettings(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.printAgent.UpdateConfig(ctx, req)
	c.JSON(http.StatusOK, s.printAgent.CurrentConfig())
}

func (s *Server) printAgentJobs(c *gin.Context) {
	if s.printAgent == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "print agent unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": s.printAgent.Jobs(50)})
}

func (s *Server) printAgentPollOnce(c *gin.Context) {
	if s.printAgent == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "print agent unavailable"})
		return
	}
	if err := s.printAgent.TriggerPoll(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
