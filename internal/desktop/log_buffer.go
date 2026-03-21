package desktop

import (
	"bytes"
	"strings"
	"sync"
	"time"
)

type LogBuffer struct {
	mu    sync.Mutex
	lines []string
	limit int
}

func NewLogBuffer(limit int) *LogBuffer {
	if limit <= 0 {
		limit = 3000
	}
	return &LogBuffer{limit: limit}
}

func (b *LogBuffer) Write(p []byte) (int, error) {
	text := strings.ReplaceAll(string(p), "\r\n", "\n")
	chunks := strings.Split(text, "\n")
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, line := range chunks {
		if line == "" {
			continue
		}
		ts := time.Now().Format("2006-01-02 15:04:05.000")
		b.lines = append(b.lines, ts+" "+line)
	}
	if extra := len(b.lines) - b.limit; extra > 0 {
		b.lines = append([]string(nil), b.lines[extra:]...)
	}
	return len(p), nil
}

func (b *LogBuffer) AllText() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.lines) == 0 {
		return "暂无日志"
	}
	var buf bytes.Buffer
	for i, line := range b.lines {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)
	}
	return buf.String()
}
