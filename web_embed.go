package webembed

import (
	"embed"
	"io/fs"
)

//go:embed all:web
var embeddedWeb embed.FS

func WebFS() (fs.FS, error) {
	return fs.Sub(embeddedWeb, "web")
}
