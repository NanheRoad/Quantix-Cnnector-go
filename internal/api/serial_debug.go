package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerSerialDebugRoutes(rg *gin.RouterGroup) {
	r := rg.Group("/api/serial-debug")
	r.GET("/ports", s.listSerialPorts)
	r.GET("/status", s.serialStatus)
	r.POST("/open", s.serialOpen)
	r.POST("/close", s.serialClose)
	r.POST("/send", s.serialSend)
	r.GET("/read", s.serialRead)
	r.GET("/logs", s.serialLogs)
}

func (s *Server) listSerialPorts(c *gin.Context) {
	ports := s.serialDebug.ListPorts(context.Background())
	c.JSON(http.StatusOK, gin.H{"ok": true, "ports": ports})
}

func (s *Server) serialStatus(c *gin.Context) {
	c.JSON(http.StatusOK, s.serialDebug.Status(context.Background()))
}

func (s *Server) serialOpen(c *gin.Context) {
	var req SerialOpenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	params := map[string]any{
		"port":       req.Port,
		"baudrate":   req.Baudrate,
		"bytesize":   req.Bytesize,
		"parity":     req.Parity,
		"stopbits":   req.Stopbits,
		"timeout_ms": req.TimeoutMS,
	}
	result, err := s.serialDebug.Open(context.Background(), params)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) serialClose(c *gin.Context) {
	c.JSON(http.StatusOK, s.serialDebug.Close(context.Background()))
}

func (s *Server) serialSend(c *gin.Context) {
	var req SerialSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	result, err := s.serialDebug.Send(context.Background(), req.Data, req.DataFormat, req.Encoding, req.LineEnding)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) serialRead(c *gin.Context) {
	maxBytes := queryInt(c, "max_bytes", 1024)
	timeoutMS := queryInt(c, "timeout_ms", 30)
	encoding := c.DefaultQuery("encoding", "utf-8")
	result, err := s.serialDebug.Read(context.Background(), maxBytes, timeoutMS, encoding)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) serialLogs(c *gin.Context) {
	lastSeq := queryInt(c, "last_seq", 0)
	limit := queryInt(c, "limit", 200)
	c.JSON(http.StatusOK, s.serialDebug.PullLogs(context.Background(), lastSeq, limit))
}

func queryInt(c *gin.Context, key string, fallback int) int {
	raw := c.Query(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
