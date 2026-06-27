package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PasswordCrypto handles AES-256-GCM encryption of the remote password.
// The AES key is stored at ~/.drift/.key (permissions 0600).
type PasswordCrypto struct {
	key []byte
}

func keyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".drift", ".key"), nil
}

// NewPasswordCrypto loads the AES key from ~/.drift/.key, or generates a
// random 32-byte key and writes it there if it doesn't exist.
func NewPasswordCrypto() (*PasswordCrypto, error) {
	path, err := keyPath()
	if err != nil {
		return nil, err
	}
	key, err := os.ReadFile(path)
	if err != nil || len(key) != 32 {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("cannot generate AES key: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("cannot create .drift directory: %w", err)
		}
		if err := os.WriteFile(path, key, 0600); err != nil {
			return nil, fmt.Errorf("cannot write AES key: %w", err)
		}
	}
	return &PasswordCrypto{key: key}, nil
}

// EncryptPassword encrypts plaintext with AES-256-GCM using a random 12-byte nonce.
// Returns "$aes$" + base64.StdEncoding(nonce + ciphertext).
// If plaintext is empty, returns empty string.
func (pc *PasswordCrypto) EncryptPassword(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(pc.key)
	if err != nil {
		return "", fmt.Errorf("cannot create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cannot create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("cannot generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	combined := append(nonce, ciphertext...)
	return "$aes$" + base64.StdEncoding.EncodeToString(combined), nil
}

// DecryptPassword reverses EncryptPassword. The input must start with "$aes$".
// If input is empty, returns empty string.
// If input does NOT start with "$aes$" (legacy plaintext), returns it as-is.
// Returns error if crypto operations fail (suggests re-running remote setup).
func (pc *PasswordCrypto) DecryptPassword(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	if !strings.HasPrefix(encoded, "$aes$") {
		return encoded, nil
	}
	data, err := base64.StdEncoding.DecodeString(encoded[5:])
	if err != nil {
		return "", fmt.Errorf("cannot decode password: %w", err)
	}
	block, err := aes.NewCipher(pc.key)
	if err != nil {
		return "", fmt.Errorf("cannot create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cannot create GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("cannot decrypt password: %w", err)
	}
	return string(plaintext), nil
}
