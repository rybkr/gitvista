package gitvista

import (
	"embed"
	"io/fs"
)

// Embedded web assets from the web/ directory.
// This allows the binary to run without external files.
//
//go:embed all:web
var embeddedFS embed.FS

// GetWebFS returns the embedded filesystem for serving static web assets.
func GetWebFS() (fs.FS, error) {
	webFS, err := fs.Sub(embeddedFS, "web")
	if err != nil {
		return nil, err
	}
	return webFS, nil
}
