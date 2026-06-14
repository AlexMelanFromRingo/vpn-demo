package session

import (
	"fmt"
	"os"
	"strings"
)

// LoadOrCreateIdentityFile loads a static identity from a base64 private-key
// file, or, if the file does not exist, generates a new identity and persists
// it with owner-only permissions. The returned bool reports whether a new key
// was created. If path is empty, an ephemeral (non-persisted) identity is
// returned — convenient for testing but unsuitable when a stable identity is
// required (server static key, allowlisted client key).
func LoadOrCreateIdentityFile(path string) (id *Identity, created bool, err error) {
	if path == "" {
		id, err = GenerateIdentity()
		return id, true, err
	}

	data, readErr := os.ReadFile(path)
	switch {
	case readErr == nil:
		id, err = LoadIdentity(strings.TrimSpace(string(data)))
		if err != nil {
			return nil, false, fmt.Errorf("parse key file %q: %w", path, err)
		}
		return id, false, nil
	case os.IsNotExist(readErr):
		id, err = GenerateIdentity()
		if err != nil {
			return nil, false, err
		}
		if err = os.WriteFile(path, []byte(id.PrivateKeyBase64()+"\n"), 0o600); err != nil {
			return nil, false, fmt.Errorf("write key file %q: %w", path, err)
		}
		return id, true, nil
	default:
		return nil, false, fmt.Errorf("read key file %q: %w", path, readErr)
	}
}
