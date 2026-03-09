// Package gitvista provides Git repository visualization with a real-time web interface.
package gitvista

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed all:web all:docs/site scripts/install.sh
var embeddedFS embed.FS

// GetWebFS returns the embedded filesystem for serving static web assets.
func GetWebFS() (fs.FS, error) {
	webFS, err := fs.Sub(embeddedFS, "web")
	if err != nil {
		return nil, err
	}
	return webFS, nil
}

// GetDocsFS returns the embedded filesystem for hosted docs source files.
func GetDocsFS() (fs.FS, error) {
	docsFS, err := fs.Sub(embeddedFS, "docs/site")
	if err != nil {
		return nil, err
	}
	return docsFS, nil
}

// GetInstallScript returns the curlable installer script served by gitvista.io.
func GetInstallScript() ([]byte, error) {
	body, err := embeddedFS.ReadFile("scripts/install.sh")
	if err != nil {
		return nil, fmt.Errorf("read install script: %w", err)
	}
	return body, nil
}
