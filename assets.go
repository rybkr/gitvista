// Package gitvista provides Git repository visualization with a real-time web interface.
package gitvista

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed all:web/*.js all:web/*.css all:web/*.png all:web/graph all:web/tooltips all:web/utils all:web/vendor all:web/images all:web/gitvista all:web/local
var localEmbeddedFS embed.FS

//go:embed all:web/*.js all:web/*.css all:web/*.png all:web/graph all:web/tooltips all:web/utils all:web/vendor all:web/images all:web/gitvista all:web/site all:web/landing all:docs/site scripts/install.sh
var siteEmbeddedFS embed.FS

// GetLocalWebFS returns the embedded filesystem for the local app shell.
func GetLocalWebFS() (fs.FS, error) {
	webFS, err := fs.Sub(localEmbeddedFS, "web")
	if err != nil {
		return nil, err
	}
	return webFS, nil
}

// GetSiteWebFS returns the embedded filesystem for the public hosted site.
func GetSiteWebFS() (fs.FS, error) {
	webFS, err := fs.Sub(siteEmbeddedFS, "web")
	if err != nil {
		return nil, err
	}
	return webFS, nil
}

// GetSiteDocsFS returns the embedded filesystem for hosted docs source files.
func GetSiteDocsFS() (fs.FS, error) {
	docsFS, err := fs.Sub(siteEmbeddedFS, "docs/site")
	if err != nil {
		return nil, err
	}
	return docsFS, nil
}

// GetInstallScript returns the curlable installer script served by gitvista.io.
func GetInstallScript() ([]byte, error) {
	body, err := siteEmbeddedFS.ReadFile("scripts/install.sh")
	if err != nil {
		return nil, fmt.Errorf("read install script: %w", err)
	}
	return body, nil
}
