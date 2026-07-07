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
// plaintext passwords (v1; a credential-helper backend may follow in v2).
const credentialFilePerm = 0o600

// Credential is a single (host, user) → password entry. The match key is
// host+user so the same NAS can have multiple accounts. Password is the only
// secret; for passwordless protocols (future SSH key, AWS keys) this struct
// will need extending, but v1 supports only password-based auth.
type Credential struct {
	Host     string `json:"host"`
	User     string `json:"user"`
	Password string `json:"password"`
}

// CredentialsFile is the on-disk format of credentials.json.
type CredentialsFile struct {
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

// FindCredential returns the password for the given host+user, or an error
// wrapping os.ErrNotExist when no matching credential is configured.
func (cf *CredentialsFile) FindCredential(host, user string) (string, error) {
	for _, c := range cf.Credentials {
		if c.Host == host && c.User == user {
			return c.Password, nil
		}
	}
	return "", fmt.Errorf("credential for %s@%s: %w", user, host, os.ErrNotExist)
}

// AddOrUpdateCredential adds c to the file, or replaces the existing entry
// with the same host+user. The returned bool is true when an existing entry
// was replaced.
func (cf *CredentialsFile) AddOrUpdateCredential(c Credential) bool {
	for i, existing := range cf.Credentials {
		if existing.Host == c.Host && existing.User == c.User {
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
