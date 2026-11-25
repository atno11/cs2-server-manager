package manager

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// Policy is supplied by the local operator, never by the instance API.
type Policy struct {
	Executables     []string `json:"executables"`
	ExecutableRoots []string `json:"executable_roots"`
	WorkRoots       []string `json:"work_roots"`
	BaseArgs        []string `json:"base_args,omitempty"`
	KnownSecrets    []string `json:"known_secrets,omitempty"`
}
type trustedFile struct {
	path     string
	dev, ino uint64
	digest   [32]byte
}
type trustPolicy struct {
	files     map[string]trustedFile
	roots     []string
	execRoots []string
	baseArgs  []string
	secrets   []string
}

func secureFile(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0077 != 0 {
		return errors.New("sensitive file must be a regular owner-only file")
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) {
		return errors.New("sensitive file has wrong owner")
	}
	return nil
}
func secureDir(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.IsDir() || fi.Mode().Perm()&0077 != 0 {
		return errors.New("runtime directory must be owner-only")
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) {
		return errors.New("runtime directory has wrong owner")
	}
	return nil
}
func ReadPolicy(path string) (Policy, error) {
	var p Policy
	if err := secureFile(path); err != nil {
		return p, err
	}
	f, err := os.Open(path)
	if err != nil {
		return p, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 16*1024+1))
	if err != nil {
		return p, err
	}
	if len(b) > 16*1024 {
		return p, errors.New("policy file too large")
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, errors.New("invalid policy file")
	}
	return p, nil
}
func canonical(path string) (string, error) {
	if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) || path != filepath.Clean(path) {
		return "", errors.New("absolute path required")
	}
	v, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if v != filepath.Clean(path) {
		return "", errors.New("symlink path not allowed")
	}
	return v, nil
}
func beneath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
func fileIdentity(path string) (trustedFile, error) {
	var out trustedFile
	f, err := os.Open(path)
	if err != nil {
		return out, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return out, err
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0111 == 0 || fi.Mode().Perm()&0022 != 0 {
		return out, errors.New("executable must be executable regular file")
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 && st.Uid != uint32(os.Geteuid()) {
		return out, errors.New("executable must be owned by root or manager user")
	}
	sum, err := hashFile(f)
	if err != nil {
		return out, err
	}
	return trustedFile{path: path, dev: uint64(st.Dev), ino: st.Ino, digest: sum}, nil
}
func hashFile(f *os.File) ([32]byte, error) {
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, err
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}
func compilePolicy(p Policy) (trustPolicy, error) {
	out := trustPolicy{files: map[string]trustedFile{}, baseArgs: slices.Clone(p.BaseArgs), secrets: slices.Clone(p.KnownSecrets)}
	if len(p.Executables) == 0 || len(p.ExecutableRoots) == 0 || len(p.WorkRoots) == 0 || len(p.Executables) > 16 || len(p.ExecutableRoots) > 16 || len(p.WorkRoots) > 16 {
		return out, errors.New("policy requires 1-16 executables and roots")
	}
	for _, root := range p.WorkRoots {
		canon, err := canonical(root)
		if err != nil {
			return out, fmt.Errorf("invalid work root: %w", err)
		}
		fi, err := os.Stat(canon)
		if err != nil || !fi.IsDir() {
			return out, errors.New("invalid work root")
		}
		st := fi.Sys().(*syscall.Stat_t)
		if st.Uid != uint32(os.Geteuid()) || fi.Mode().Perm()&0022 != 0 {
			return out, errors.New("work root must be owned by manager user and not group/world writable")
		}
		out.roots = append(out.roots, canon)
	}
	for _, root := range p.ExecutableRoots {
		canon, err := canonical(root)
		if err != nil {
			return out, fmt.Errorf("invalid executable root: %w", err)
		}
		fi, err := os.Stat(canon)
		if err != nil || !fi.IsDir() {
			return out, errors.New("invalid executable root")
		}
		st := fi.Sys().(*syscall.Stat_t)
		if fi.Mode().Perm()&0022 != 0 || st.Uid != 0 && st.Uid != uint32(os.Geteuid()) {
			return out, errors.New("executable root is not trusted")
		}
		out.execRoots = append(out.execRoots, canon)
	}
	for _, path := range p.Executables {
		canon, err := canonical(path)
		if err != nil {
			return out, fmt.Errorf("invalid trusted executable: %w", err)
		}
		allowed := false
		for _, root := range out.execRoots {
			if beneath(root, canon) {
				if err := secureExecTree(root, canon); err != nil {
					return out, err
				}
				allowed = true
				break
			}
		}
		if !allowed {
			return out, errors.New("trusted executable outside executable roots")
		}
		id, err := fileIdentity(canon)
		if err != nil {
			return out, err
		}
		out.files[canon] = id
	}
	if len(p.BaseArgs) > 64 {
		return out, errors.New("too many base arguments")
	}
	for _, a := range p.BaseArgs {
		if len(a) == 0 || len(a) > 1024 || containsNUL(a) || protectedPortArg(a) {
			return out, errors.New("invalid base argument")
		}
	}
	if len(p.KnownSecrets) > 64 {
		return out, errors.New("too many known secrets")
	}
	for _, v := range p.KnownSecrets {
		if len(v) < 4 || len(v) > 256 {
			return out, errors.New("known secret length must be 4-256")
		}
	}
	return out, nil
}
func protectedPortArg(a string) bool {
	v := strings.ToLower(a)
	return strings.HasPrefix(v, "-port") || strings.HasPrefix(v, "+hostport")
}
func secureExecTree(root, path string) error {
	current := root
	rel, _ := filepath.Rel(root, filepath.Dir(path))
	if rel == "." {
		return nil
	}
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		current = filepath.Join(current, part)
		fi, err := os.Lstat(current)
		if err != nil {
			return err
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok || !fi.IsDir() || fi.Mode().Perm()&0022 != 0 || st.Uid != 0 && st.Uid != uint32(os.Geteuid()) {
			return errors.New("executable tree is not trusted")
		}
	}
	return nil
}
func secureWorkTree(root, path string) error {
	current := root
	rel, _ := filepath.Rel(root, path)
	parts := []string{}
	if rel != "." {
		parts = strings.Split(rel, string(os.PathSeparator))
	}
	for _, part := range parts {
		current = filepath.Join(current, part)
		fi, err := os.Lstat(current)
		if err != nil {
			return err
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != uint32(os.Geteuid()) || !fi.IsDir() || fi.Mode().Perm()&0022 != 0 {
			return errors.New("working directory tree is not private to manager user")
		}
	}
	return nil
}
func (p trustPolicy) validateDefinition(d Definition) (Definition, error) {
	canon, err := canonical(d.Executable)
	if err != nil {
		return d, err
	}
	if _, ok := p.files[canon]; !ok {
		return d, errors.New("executable not trusted")
	}
	work, err := canonical(d.WorkDir)
	if err != nil {
		return d, err
	}
	allowed := false
	for _, root := range p.roots {
		if beneath(root, work) {
			if err := secureWorkTree(root, work); err != nil {
				return d, err
			}
			allowed = true
			break
		}
	}
	if !allowed {
		return d, errors.New("working directory outside trusted roots")
	}
	fi, err := os.Stat(work)
	if err != nil || !fi.IsDir() {
		return d, errors.New("invalid working directory")
	}
	if fi.Mode().Perm()&0022 != 0 {
		return d, errors.New("working directory is group/world writable")
	}
	d.Executable = canon
	d.WorkDir = work
	return d, nil
}
func (p trustPolicy) openExecutable(path string) (*os.File, error) {
	expected, ok := p.files[path]
	if !ok {
		return nil, errors.New("executable not trusted")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || uint64(st.Dev) != expected.dev || st.Ino != expected.ino || !fi.Mode().IsRegular() || fi.Mode().Perm()&0111 == 0 {
		f.Close()
		return nil, errors.New("trusted executable changed")
	}
	sum, err := hashFile(f)
	if err != nil || sum != expected.digest {
		f.Close()
		return nil, errors.New("trusted executable changed")
	}
	return f, nil
}
