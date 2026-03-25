// Package gitvista provides Git repository visualization with a real-time web interface.
package gitvista

import (
	"embed"
	"io/fs"
)

//go:embed all:web/*.js all:web/*.css all:web/*.png all:web/graph all:web/tooltips all:web/utils all:web/vendor all:web/images all:web/gitvista all:web/local
var localEmbeddedFS embed.FS

// GetLocalWebFS returns the embedded filesystem for the local app shell.
func GetLocalWebFS() (fs.FS, error) {
	webFS, err := fs.Sub(localEmbeddedFS, "web")
	if err != nil {
		return nil, err
	}
	return webFS, nil
}
