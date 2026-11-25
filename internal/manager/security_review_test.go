package manager

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
)

func TestReviewOverlappingKnownSecrets(t *testing.T) {
	cases := []struct {
		name    string
		secrets []string
		input   string
	}{
		{"overlap-forward", []string{"abcde", "cdefg"}, "abcdefg"},
		{"overlap-reverse", []string{"cdefg", "abcde"}, "abcdefg"},
		{"nested-forward", []string{"abcd", "abcdef"}, "abcdef"},
		{"nested-reverse", []string{"abcdef", "abcd"}, "abcdef"},
		{"adjacent", []string{"abcd", "efgh"}, "abcdefgh"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRing(tc.secrets...)
			for _, part := range []string{tc.input[:2], tc.input[2:]} {
				_, _ = r.Write([]byte(part))
			}
			r.Flush()
			if got := r.String(); got != "[REDACTED]" {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestReviewRootAndExecutableMetadataRecheck(t *testing.T) {
	m, d, root := policyFixture(t)
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0777); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(d.ID); err == nil {
		t.Fatal("writable trusted roots accepted")
	}
	_ = os.Chmod(root, 0700)
	if err := os.Chmod(d.Executable, 0722); err != nil {
		t.Fatal(err)
	}
	if _, err := m.policy.openExecutable(d.Executable); err == nil {
		t.Fatal("writable executable accepted")
	}
}

func TestReviewPersistedDefinitionCountAndDuplicateJSON(t *testing.T) {
	m, d, root := policyFixture(t)
	dataDir := filepath.Join(root, "legacy-data")
	if err := os.Mkdir(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dataDir, "instances.json")
	defs := make([]Definition, 33)
	for i := range defs {
		defs[i] = d
		defs[i].ID = string(rune('a'+i/26)) + string(rune('a'+i%26))
		defs[i].Port = 27015 + i
		defs[i].WorkDir = filepath.Join(root, defs[i].ID)
		if err := os.Mkdir(defs[i].WorkDir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := json.Marshal(defs)
	if err := os.WriteFile(state, b, 0600); err != nil {
		t.Fatal(err)
	}
	policy := Policy{Executables: []string{d.Executable}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}
	if _, err := newWithPreflight(dataDir, policy, func() error { return nil }); err == nil {
		t.Fatal("33 persisted definitions accepted")
	}
	if err := os.WriteFile(state, []byte(`[ {"id":"one","ID":"two","executable":"`+d.Executable+`","work_dir":"`+d.WorkDir+`","port":27015} ]`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := newWithPreflight(dataDir, policy, func() error { return nil }); err == nil {
		t.Fatal("ambiguous persisted definition accepted")
	}
	_ = m
}

func TestReviewKnownSecretInID(t *testing.T) {
	m, d, _ := policyFixture(t)
	m.policy.secrets = []string{"secret"}
	d.ID = "secret-instance"
	if err := m.Register(d); err == nil {
		t.Fatal("known secret in public ID accepted")
	}
}

func TestReviewPolicyDuplicateFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "policy.json")
	if err := os.WriteFile(path, []byte(`{"work_roots":["/tmp"],"WORK_ROOTS":["/etc"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPolicy(path); err == nil {
		t.Fatal("ambiguous policy accepted")
	}
}

func TestReviewPendingBound(t *testing.T) {
	r := newRing(strings.Repeat("s", 256))
	_, _ = r.Write([]byte(strings.Repeat("a", logLimit*2)))
	if len(r.pending) > 256 || len(r.String()) > logLimit {
		t.Fatal("unbounded redaction memory")
	}
}

func TestReviewRootMetadataAndPrivilegeModes(t *testing.T) {
	m, d, root := policyFixture(t)
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := m.policy.openExecutable(d.Executable); err == nil {
		t.Fatal("writable executable root accepted")
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []os.FileMode{0700 | os.ModeSetuid, 0700 | os.ModeSetgid, 0722} {
		if err := os.Chmod(d.Executable, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := m.policy.openExecutable(d.Executable); err == nil {
			t.Fatalf("unsafe mode %o accepted", mode)
		}
	}
	if err := os.Chmod(d.Executable, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, d.ID, "stopped")
}

func TestReviewFileCapabilityInspection(t *testing.T) {
	_, d, _ := policyFixture(t)
	f, err := os.Open(d.Executable)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, tc := range []struct {
		name     string
		get      func(string, string, []byte) (int, error)
		rejected bool
	}{
		{"capability-present", func(_, _ string, _ []byte) (int, error) { return 4, nil }, true},
		{"inspection-error", func(_, _ string, _ []byte) (int, error) { return 0, syscall.EPERM }, true},
		{"no-capability", func(_, _ string, _ []byte) (int, error) { return 0, syscall.ENODATA }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := rejectFileCapabilitiesWith(f, tc.get)
			if (err != nil) != tc.rejected {
				t.Fatalf("err=%v rejected=%v", err, tc.rejected)
			}
		})
	}
}

func TestReviewPersistedSizeAndNoRewrite(t *testing.T) {
	_, d, root := policyFixture(t)
	dataDir := filepath.Join(root, "legacy-data")
	if err := os.Mkdir(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dataDir, "instances.json")
	policy := Policy{Executables: []string{d.Executable}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}
	for _, data := range [][]byte{[]byte(strings.Repeat("x", 512*1024+1)), []byte(`[ {"id":"one","executable":"` + d.Executable + `","work_dir":"` + d.WorkDir + `","port":27015,"args":["` + strings.Repeat("a", 16*1024) + `"]} ]`)} {
		if err := os.WriteFile(state, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := newWithPreflight(dataDir, policy, func() error { return nil }); err == nil {
			t.Fatal("oversized persisted state accepted")
		}
		after, err := os.ReadFile(state)
		if err != nil || !slices.Equal(after, data) {
			t.Fatal("invalid state changed")
		}
	}
}

func TestReviewPersistedFormattedDefinition(t *testing.T) {
	_, d, root := policyFixture(t)
	dataDir := filepath.Join(root, "legacy-data")
	if err := os.Mkdir(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dataDir, "instances.json")
	body := []byte(`[{` + strings.Repeat(" ", 20*1024) + `"id":"one","executable":"` + d.Executable + `","work_dir":"` + d.WorkDir + `","port":27015}]`)
	if err := os.WriteFile(state, body, 0600); err != nil {
		t.Fatal(err)
	}
	policy := Policy{Executables: []string{d.Executable}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}
	m, err := newWithPreflight(dataDir, policy, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if len(m.List()) != 1 {
		t.Fatal("formatted valid definition lost")
	}
}

func TestReviewSecretLimitsAndPublicConstants(t *testing.T) {
	_, d, root := policyFixture(t)
	base := Policy{Executables: []string{d.Executable}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}
	for _, count := range []int{0, 64, 65} {
		p := base
		for i := 0; i < count; i++ {
			p.KnownSecrets = append(p.KnownSecrets, fmt.Sprintf("synthetic-secret-%03d", i))
		}
		p.InternalSecrets = []string{strings.Repeat("t", 64)}
		_, err := compilePolicy(p)
		if (err == nil) != (count <= 64) {
			t.Fatalf("count=%d err=%v", count, err)
		}
	}
	p := base
	p.KnownSecrets = []string{"running"}
	if _, err := compilePolicy(p); err == nil {
		t.Fatal("public state secret accepted")
	}
}

func TestReviewDescriptorHelper(t *testing.T) {
	if os.Getenv("CS2M_DESCRIPTOR_HELPER") != "1" {
		return
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink("/proc/self/exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("/proc/self/fd/3"); err != nil {
		t.Fatal(err)
	}
	resource, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "resource.txt"))
	if err != nil {
		t.Fatal(err)
	}
	data := fmt.Sprintf("argv0=%s\nexe=%s\nwd=%s\nresource=%s\n", os.Args[0], target, wd, resource)
	if err := os.WriteFile("descriptor-observation", []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestReviewDescriptorExecution(t *testing.T) {
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	exe := filepath.Join(root, "trusted.test")
	in, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(exe, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "resource.txt"), []byte("trusted-resource"), 0600); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CS2M_DESCRIPTOR_HELPER", "1")
	m, err := New(filepath.Join(work, "data"), Policy{Executables: []string{exe}, ExecutableRoots: []string{root}, WorkRoots: []string{work}, BaseArgs: []string{"-test.run=^TestReviewDescriptorHelper$", "--"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	d := Definition{ID: "descriptor", Executable: exe, WorkDir: work, Port: 27015}
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, "descriptor observation", func() bool { _, err := os.Stat(filepath.Join(work, "descriptor-observation")); return err == nil })
	data, err := os.ReadFile(filepath.Join(work, "descriptor-observation"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"argv0=" + exe, "exe=" + exe, "wd=" + work, "resource=trusted-resource"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %q in %q", want, data)
		}
	}
}
