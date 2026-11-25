package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func checkPrivate(path string, dir bool) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if (dir && !fi.IsDir()) || (!dir && !fi.Mode().IsRegular()) || fi.Mode().Perm()&0077 != 0 {
		return errors.New("credential path has unsafe ownership or permissions")
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) {
		return errors.New("credential path has unsafe ownership or permissions")
	}
	return nil
}

// LoadOrCreateToken bootstraps an owner-only 256-bit bearer credential.
func LoadOrCreateToken(dir string) (string, error) {
	if err := checkPrivate(dir, true); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "admin.token")
	if err := checkPrivate(path, false); err == nil {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if len(b) != 64 {
			return "", errors.New("invalid credential file")
		}
		for _, c := range b {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return "", errors.New("invalid credential file")
			}
		}
		return string(b), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	if _, err = f.WriteString(token); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	return token, nil
}
func EqualToken(got, want string) bool {
	if len(got) != len(want) || len(want) != 64 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
