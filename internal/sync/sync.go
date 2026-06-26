// Package sync provides remote synchronization for drift projects.
//
// The sync engine supports multiple transports (WebDAV, FTP, SFTP, SMB)
// behind a common Transport interface. Synchronization is incremental
// (content-hash based) and tracks deletions via a manifest file stored on
// the remote. Auto-sync is triggered after 'drift save'.
package sync

import (
	"fmt"
	"strings"

	"github.com/drift/drift/internal/config"
)

// RemoteType indicates what kind of remote is configured.
type RemoteType int

const (
	RemoteNone RemoteType = iota
	RemoteWebDAV
	RemoteFTP
	RemoteSFTP
	RemoteSMB
)

// GetRemoteType returns the configured remote type based on the Protocol field.
func GetRemoteType(g *config.GlobalConfig) RemoteType {
	switch g.Protocol {
	case "webdav":
		return RemoteWebDAV
	case "ftp":
		return RemoteFTP
	case "sftp":
		return RemoteSFTP
	case "smb":
		return RemoteSMB
	default:
		return RemoteNone
	}
}

func defaultPort(protocol string) int {
	switch protocol {
	case "ftp":
		return 21
	case "sftp":
		return 22
	case "smb":
		return 445
	case "webdav":
		return 80
	default:
		return 0
	}
}

// EffectivePort returns the configured port or the protocol default.
func EffectivePort(g *config.GlobalConfig) int {
	if g.Port != 0 {
		return g.Port
	}
	if g.Protocol == "webdav" && g.TLS {
		return 443
	}
	return defaultPort(g.Protocol)
}

func webDAVBaseURL(g *config.GlobalConfig) string {
	scheme := "http"
	if g.TLS {
		scheme = "https"
	}
	basePath := strings.Trim(g.Path, "/")
	port := EffectivePort(g)
	if basePath != "" {
		return fmt.Sprintf("%s://%s:%d/%s", scheme, g.Host, port, basePath)
	}
	return fmt.Sprintf("%s://%s:%d", scheme, g.Host, port)
}

// ProjectTransportForConfig returns a Transport scoped to a project, based
// on the global config. The caller must call Close() when done.
func ProjectTransportForConfig(gcfg *config.GlobalConfig, remoteName string) (Transport, error) {
	switch GetRemoteType(gcfg) {
	case RemoteWebDAV:
		baseURL := webDAVBaseURL(gcfg) + "/" + remoteName
		return NewWebDAVTransport(baseURL, gcfg.Username, gcfg.Password, gcfg.InsecureSkipVerify), nil
	case RemoteFTP:
		return NewFTPTransport(gcfg, remoteName)
	case RemoteSFTP:
		return NewSFTPTransport(gcfg, remoteName)
	case RemoteSMB:
		return NewSMBTransport(gcfg, remoteName)
	}
	return nil, fmt.Errorf("no remote configured")
}
