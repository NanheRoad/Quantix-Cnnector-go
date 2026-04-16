//go:build !windows

package api

import (
	"fmt"
	"runtime"
)

func openLocalFileDialog(_, _ string) (string, error) {
	return "", fmt.Errorf("local file picker not supported on %s", runtime.GOOS)
}
