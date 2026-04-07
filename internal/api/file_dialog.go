package api

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
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

func openLocalFileDialog(title, filter string) (string, error) {
	switch runtime.GOOS {
	case "windows":
		return openWindowsFileDialog(title, filter)
	default:
		return "", fmt.Errorf("local file picker not supported on %s", runtime.GOOS)
	}
}

func openWindowsFileDialog(title, filter string) (string, error) {
	if title == "" {
		title = "选择文件"
	}
	if filter == "" {
		filter = "All files (*.*)|*.*"
	}
	title = strings.ReplaceAll(title, "'", "''")
	filter = strings.ReplaceAll(filter, "'", "''")
	script := strings.Join([]string{
		"Add-Type -AssemblyName System.Windows.Forms",
		"$dialog = New-Object System.Windows.Forms.OpenFileDialog",
		fmt.Sprintf("$dialog.Title = '%s'", title),
		fmt.Sprintf("$dialog.Filter = '%s'", filter),
		"$dialog.CheckFileExists = $true",
		"$dialog.Multiselect = $false",
		"$result = $dialog.ShowDialog()",
		"if ($result -eq [System.Windows.Forms.DialogResult]::OK) {",
		"  [Console]::OutputEncoding = [System.Text.Encoding]::UTF8",
		"  Write-Output $dialog.FileName",
		"}",
	}, "; ")
	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-Command", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("open file dialog failed: %s", strings.TrimSpace(string(out)))
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("no file selected")
	}
	return path, nil
}
