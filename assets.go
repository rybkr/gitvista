// Package gitvista provides Git repository visualization with a real-time web interface.
package gitvista

import (
	"embed"
	"io/fs"
)

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
