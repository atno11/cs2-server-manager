package tui

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

var errCredential = errors.New("credential file is missing or unsafe")

// LoadToken reads an existing private credential without following a final
// symlink. The daemon remains the sole credential creator.
func LoadToken(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", errCredential
	}
	dir := filepath.Dir(path)
	if err := privatePath(dir, true); err != nil {
		return "", errCredential
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", errCredential
	}
	f := os.NewFile(uintptr(fd), "credential")
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !privateInfo(fi, false) || fi.Size() != 64 {
		return "", errCredential
	}
	b, err := io.ReadAll(io.LimitReader(f, 65))
	if err != nil || len(b) != 64 {
		return "", errCredential
	}
	for _, c := range b {
		if c < '0' || c > '9' && c < 'a' || c > 'f' {
			return "", errCredential
		}
	}
	return string(b), nil
}

func privatePath(path string, dir bool) error {
	fi, err := os.Lstat(path)
	if err != nil || !privateInfo(fi, dir) {
		return errCredential
	}
	return nil
}

func privateInfo(fi os.FileInfo, dir bool) bool {
	if fi == nil || fi.Mode().Perm()&0077 != 0 || (dir && !fi.IsDir()) || (!dir && !fi.Mode().IsRegular()) {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && st.Uid == uint32(os.Geteuid())
}
