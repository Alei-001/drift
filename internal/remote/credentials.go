package remote

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/Alei-001/drift/internal/util/fsutil"
)

// credentialFilePerm is the file mode for credentials.json. 0600 ensures
// only the owner can read the file, which is critical since it contains
// plaintext passwords.
//
// Security note: credentials are currently stored in plaintext. This is
// acceptable for v1 (matching git's credential-store approach), but a
// future version should integrate with the OS keyring / secret store
// (e.g. keyring-go, dbus Secret Service, Windows Credential Manager)
// to avoid leaving passwords readable on disk. Until then, the 0600
// permission is the only protection — do not weaken it.
//
// Callers must never include Password in error messages or log output.
const credentialFilePerm = 0o600

// Credential is a remote-name → password entry. Using the remote name as
// the key (rather than host+user) avoids collisions when the same host+user
// is used with different passwords for different remotes (e.g. work vs
// personal projects on the same NAS).
//
// Password is stored in plaintext in credentials.json (protected only by
// 0600 file permissions). See credentialFilePerm for the keyring roadmap.
type Credential struct {
	Remote   string `json:"remote"`
	Host     string `json:"host"`
	User     string `json:"user"`
	Password string `json:"password"`
}

// CredentialsFile is the on-disk format of credentials.json. The Version
// field supports future format migrations; the current on-disk version is 1.
// A missing version (zero value) indicates a legacy file written before the
// field was introduced and is read as-is.
type CredentialsFile struct {
	Version     int          `json:"version,omitempty"`
	Credentials []Credential `json:"credentials"`
}

// credentialsPath returns the path to the user-level credentials.json:
// <UserConfigDir>/drift/credentials.json. The directory is created if it
// does not exist.
func credentialsPath() (string, error) {
	userCfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	dir := filepath.Join(userCfgDir, "drift")
	if err := os.MkdirAll(dir, fsutil.DefaultDirPerm); err != nil {
		return "", fmt.Errorf("create user config dir: %w", err)
	}
	return filepath.Join(dir, "credentials.json"), nil
}

// LoadCredentials reads the user-level credentials.json. Returns an empty
// CredentialsFile (not an error) when the file does not exist.
func LoadCredentials() (*CredentialsFile, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &CredentialsFile{Credentials: []Credential{}}, nil
		}
		return nil, fmt.Errorf("read credentials.json: %w", err)
	}
	var cf CredentialsFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parse credentials.json: %w", err)
	}
	if cf.Credentials == nil {
		cf.Credentials = []Credential{}
	}
	return &cf, nil
}

// SaveCredentials writes the user-level credentials.json with 0600 perms.
// The write is atomic (temp file + rename).
func SaveCredentials(cf *CredentialsFile) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials.json: %w", err)
	}
	return fsutil.WriteFileAtomic(path, data, credentialFilePerm)
}

// FindCredential returns the password for the given remote name, or an error
// wrapping os.ErrNotExist when no matching credential is configured.
func (cf *CredentialsFile) FindCredential(remoteName string) (string, error) {
	for _, c := range cf.Credentials {
		if c.Remote == remoteName {
			return c.Password, nil
		}
	}
	return "", fmt.Errorf("credential for remote %q: %w", remoteName, os.ErrNotExist)
}

// AddOrUpdateCredential adds c to the file, or replaces the existing entry
// with the same remote name. The returned bool is true when an existing entry
// was replaced.
func (cf *CredentialsFile) AddOrUpdateCredential(c Credential) bool {
	for i, existing := range cf.Credentials {
		if existing.Remote == c.Remote {
			cf.Credentials[i] = c
			return true
		}
	}
	cf.Credentials = append(cf.Credentials, c)
	return false
}

// HostFromURL extracts the host portion from a remote URL. Works for both
// webdav (https://host[:port]/path) and smb (smb://host[:port]/share) URLs.
func HostFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse url %q: %w", rawURL, err)
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("url %q has no host", rawURL)
	}
	return u.Hostname(), nil
}
