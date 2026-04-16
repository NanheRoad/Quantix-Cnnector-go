package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type filePickRequest struct {
	Title  string `json:"title"`
	Filter string `json:"filter"`
}

func (s *Server) registerLocalFileRoutes(rg *gin.RouterGroup) {
	r := rg.Group("/api/local-files")
	r.POST("/pick", s.pickLocalFile)
}

func (s *Server) pickLocalFile(c *gin.Context) {
	var req filePickRequest
	_ = c.ShouldBindJSON(&req)

	path, err := openLocalFileDialog(strings.TrimSpace(req.Title), strings.TrimSpace(req.Filter))
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "not supported") {
			status = http.StatusNotImplemented
		}
		c.JSON(status, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": path})
}
