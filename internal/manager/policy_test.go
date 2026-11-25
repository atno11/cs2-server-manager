package manager

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func policyFixture(t *testing.T) (*Manager, Definition, string) {
	t.Helper()
	root := t.TempDir()
	exe := filepath.Join(root, "trusted.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho trusted\n"), 0700); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0700); err != nil {
		t.Fatal(err)
	}
	m, err := newWithPreflight(filepath.Join(root, "data"), Policy{Executables: []string{exe}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, Definition{ID: "one", Executable: exe, WorkDir: work, Port: 27015}, root
}
func TestExecutionTrustAndPathIdentity(t *testing.T) {
	m, d, root := policyFixture(t)
	bad := d
	bad.Executable = "/bin/sh"
	if err := m.Register(bad); err == nil {
		t.Fatal("untrusted executable accepted")
	}
	outside := t.TempDir()
	bad = d
	bad.WorkDir = outside
	if err := m.Register(bad); err == nil {
		t.Fatal("outside workdir accepted")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(d.WorkDir, link); err != nil {
		t.Fatal(err)
	}
	bad = d
	bad.WorkDir = link
	if err := m.Register(bad); err == nil {
		t.Fatal("symlink workdir accepted")
	}
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	alias := d
	alias.ID = "two"
	alias.Port++
	alias.WorkDir = filepath.Join(d.WorkDir, "..", "work")
	if err := m.Register(alias); err == nil {
		t.Fatal("equivalent directory accepted")
	}
	if err := os.Rename(d.WorkDir, d.WorkDir+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(d.WorkDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(d.ID); err == nil {
		t.Fatal("replaced workdir accepted")
	}
}
func TestTrustedExecutableMutationRejected(t *testing.T) {
	m, d, _ := policyFixture(t)
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.Executable, []byte("#!/bin/sh\necho changed\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(d.ID); err == nil {
		t.Fatal("changed executable accepted")
	}
}
func TestArgumentAndInstanceLimits(t *testing.T) {
	m, d, root := policyFixture(t)
	for _, args := range [][]string{{"-port=1"}, {"+hostport", "1"}, {"+exec", "bad.cfg"}, {"safe;quit"}, {strings.Repeat("a", 257)}, {"\n"}} {
		bad := d
		bad.Args = args
		if err := m.Register(bad); err == nil {
			t.Fatalf("argument accepted: %q", args)
		}
	}
	many := d
	many.Args = make([]string, 33)
	for i := range many.Args {
		many.Args[i] = "safe"
	}
	if err := m.Register(many); err == nil {
		t.Fatal("too many args accepted")
	}
	for i := 0; i < 32; i++ {
		x := d
		x.ID = string(rune('a'+i/26)) + string(rune('a'+i%26))
		x.Port = 27015 + i
		x.WorkDir = filepath.Join(root, x.ID)
		if err := os.Mkdir(x.WorkDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := m.Register(x); err != nil {
			t.Fatal(err)
		}
	}
	extra := d
	extra.ID = "extra"
	extra.Port = 30000
	extra.WorkDir = filepath.Join(root, "extra")
	if err := os.Mkdir(extra.WorkDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := m.Register(extra); err == nil {
		t.Fatal("instance limit accepted")
	}
}
func TestKnownSecretRedactionAcrossWrites(t *testing.T) {
	r := newRing("known-secret")
	for _, part := range []string{"prefix known-", "secret suffix"} {
		_, _ = r.Write([]byte(part))
		if strings.Contains(r.String(), "known-secret") {
			t.Fatal("secret exposed before flush")
		}
	}
	r.Flush()
	out := r.String()
	if strings.Contains(out, "known-secret") || !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("redaction failed: %q", out)
	}
	_, _ = r.Write([]byte(strings.Repeat("x", logLimit+100)))
	r.Flush()
	if len(r.String()) != logLimit {
		t.Fatal("log bound lost")
	}
}
func TestRuntimeAndPersistedPermissions(t *testing.T) {
	m, d, root := policyFixture(t)
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(root, "data", "instances.json"))
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatal("state file not private")
	}
	if err := os.Chmod(filepath.Join(root, "data", "instances.json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = newWithPreflight(filepath.Join(root, "data"), Policy{Executables: []string{d.Executable}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}, func() error { return nil })
	if err == nil {
		t.Fatal("unsafe state file accepted")
	}
	if err := os.Chmod(filepath.Join(root, "data"), 0755); err != nil {
		t.Fatal(err)
	}
	_, err = newWithPreflight(filepath.Join(root, "data"), Policy{Executables: []string{d.Executable}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}, func() error { return nil })
	if err == nil {
		t.Fatal("unsafe runtime directory accepted")
	}
}
func TestCapabilityRejectionBeforeManagerCreation(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "trusted.sh")
	marker := filepath.Join(root, "launched")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	_, err := newWithPreflight(filepath.Join(root, "data"), Policy{Executables: []string{exe}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}, func() error { return errors.New("unsupported") })
	if err == nil {
		t.Fatal("unsupported capability accepted")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("managed executable launched")
	}
	if err := checkLinuxCapabilitiesWithProbe("/nonexistent-cs2m-probe"); err == nil {
		t.Fatal("invalid probe accepted")
	}
}

func TestSymlinkEscapeAndPolicyFilePermissions(t *testing.T) {
	m, d, root := policyFixture(t)
	outside := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	d.WorkDir = link
	if err := m.Register(d); err == nil {
		t.Fatal("symlink escape accepted")
	}
	path := filepath.Join(root, "data", "policy.json")
	if err := os.WriteFile(path, []byte(`{"executables":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPolicy(path); err == nil {
		t.Fatal("unsafe operator policy accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPolicy(path); err != nil {
		t.Fatal(err)
	}
}
func TestLongKnownSecretSplitRedaction(t *testing.T) {
	secret := strings.Repeat("a", 64)
	r := newRing(secret)
	_, _ = r.Write([]byte("before " + secret[:31]))
	if strings.Contains(r.String(), secret) {
		t.Fatal("partial secret exposed")
	}
	_, _ = r.Write([]byte(secret[31:] + " after"))
	r.Flush()
	out := r.String()
	if strings.Contains(out, secret) || !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("split redaction failed: %q", out)
	}
}
