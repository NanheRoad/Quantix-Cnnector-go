package api

import (
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) openapi(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"openapi": "3.0.0",
		"info": gin.H{
			"title":   "Quantix Connector",
			"version": "1.0.0",
		},
		"servers": []gin.H{{"url": "http://127.0.0.1:8000"}},
		"paths": gin.H{
			"/health": gin.H{
				"get": gin.H{"summary": "Health probe with runtime and latency metrics"},
			},
			"/ws": gin.H{
				"get": gin.H{"summary": "WebSocket realtime stream (api_key query required)"},
			},
			"/api/devices":                               gin.H{"get": gin.H{}, "post": gin.H{}},
			"/api/devices/{device_id}":                   gin.H{"get": gin.H{}, "put": gin.H{}, "delete": gin.H{}},
			"/api/devices/{device_id}/enable":            gin.H{"post": gin.H{}},
			"/api/devices/{device_id}/disable":           gin.H{"post": gin.H{}},
			"/api/devices/{device_id}/execute":           gin.H{"post": gin.H{}},
			"/api/devices/by-code/{device_code}":         gin.H{"get": gin.H{}, "put": gin.H{}, "delete": gin.H{}},
			"/api/devices/by-code/{device_code}/enable":  gin.H{"post": gin.H{}},
			"/api/devices/by-code/{device_code}/disable": gin.H{"post": gin.H{}},
			"/api/devices/by-code/{device_code}/execute": gin.H{"post": gin.H{}},
			"/api/protocols":                             gin.H{"get": gin.H{}, "post": gin.H{}},
			"/api/protocols/import":                      gin.H{"post": gin.H{}},
			"/api/protocols/{protocol_id}":               gin.H{"get": gin.H{}, "put": gin.H{}, "delete": gin.H{}},
			"/api/protocols/{protocol_id}/export":        gin.H{"get": gin.H{}},
			"/api/protocols/{protocol_id}/test":          gin.H{"post": gin.H{}},
			"/api/protocols/{protocol_id}/test-step":     gin.H{"post": gin.H{}},
			"/api/printers/{device_id}/print":            gin.H{"post": gin.H{}},
			"/api/scanners/{device_id}/last":             gin.H{"get": gin.H{}},
			"/api/boards/{device_id}/status":             gin.H{"get": gin.H{}},
			"/api/serial-debug/ports":                    gin.H{"get": gin.H{}},
			"/api/serial-debug/status":                   gin.H{"get": gin.H{}},
			"/api/serial-debug/open":                     gin.H{"post": gin.H{}},
			"/api/serial-debug/close":                    gin.H{"post": gin.H{}},
			"/api/serial-debug/send":                     gin.H{"post": gin.H{}},
			"/api/serial-debug/read":                     gin.H{"get": gin.H{}},
			"/api/serial-debug/logs":                     gin.H{"get": gin.H{}},
			"/api/remote-print/jobs":                     gin.H{"post": gin.H{"summary": "Direct remote print job"}},
		},
	})
}

func (s *Server) docsPage(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<!doctype html>
<html><head><meta charset="utf-8"><title>Quantix API Docs</title>
<style>body{font-family:Arial,sans-serif;margin:24px}code{background:#f4f4f4;padding:2px 6px;border-radius:4px}</style>
</head><body>
<h1>Quantix Connector API</h1>
<p>OpenAPI JSON: <a href="/openapi.json">/openapi.json</a></p>
<p>Auth: <code>X-API-Key</code> header or <code>api_key</code> query.</p>
</body></html>`))
}

func (s *Server) serveIndex(c *gin.Context) {
	if s.webFS != nil {
		body, err := fs.ReadFile(s.webFS, "index.html")
		if err == nil {
			c.Data(http.StatusOK, "text/html; charset=utf-8", body)
			return
		}
	}
	c.File("web/index.html")
}
