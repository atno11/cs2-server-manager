package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"syscall"
	"time"
)

type Definition struct {
	ID         string   `json:"id"`
	Executable string   `json:"executable"`
	WorkDir    string   `json:"work_dir"`
	Port       int      `json:"port"`
	Args       []string `json:"args"`
}
type Instance struct {
	Definition
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}
type process struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan struct{}
}
type entry struct {
	def        Definition
	state, err string
	proc       *process
	log        *ring
}
type Manager struct {
	mu      sync.Mutex
	entries map[string]*entry
	path    string
	closed  bool
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func New(dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	m := &Manager{entries: map[string]*entry{}, path: filepath.Join(dir, "instances.json")}
	b, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	var defs []Definition
	if err = json.Unmarshal(b, &defs); err != nil {
		return nil, err
	}
	for _, d := range defs {
		if err = m.validate(d, ""); err != nil {
			return nil, fmt.Errorf("stored %s: %w", d.ID, err)
		}
		m.entries[d.ID] = &entry{def: d, state: "stopped", log: newRing()}
	}
	return m, nil
}
func (m *Manager) validate(d Definition, except string) error {
	if !idPattern.MatchString(d.ID) {
		return errors.New("invalid ID")
	}
	if d.Port < 1 || d.Port > 65535 {
		return errors.New("invalid port")
	}
	if !filepath.IsAbs(d.Executable) || !filepath.IsAbs(d.WorkDir) {
		return errors.New("paths must be absolute")
	}
	fi, err := os.Stat(d.Executable)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0111 == 0 {
		return errors.New("executable must be an existing executable file")
	}
	fi, err = os.Stat(d.WorkDir)
	if err != nil || !fi.IsDir() {
		return errors.New("working directory must exist")
	}
	for _, a := range d.Args {
		if len(a) > 1024 || len(a) == 0 || containsNUL(a) {
			return errors.New("invalid argument")
		}
		if a == "-port" || a == "+hostport" {
			return errors.New("port override is forbidden")
		}
	}
	for id, e := range m.entries {
		if id != except && (id == d.ID || e.def.Port == d.Port || e.def.WorkDir == d.WorkDir) {
			return errors.New("duplicate ID, port, or working directory")
		}
	}
	return nil
}
func containsNUL(s string) bool {
	for _, c := range s {
		if c == 0 {
			return true
		}
	}
	return false
}
func (m *Manager) save() error {
	defs := make([]Definition, 0, len(m.entries))
	for _, e := range m.entries {
		defs = append(defs, e.def)
	}
	slices.SortFunc(defs, func(a, b Definition) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	b, err := json.MarshalIndent(defs, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(m.path), ".instances-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), m.path)
}
func (m *Manager) Register(d Definition) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("manager closed")
	}
	if err := m.validate(d, ""); err != nil {
		return err
	}
	m.entries[d.ID] = &entry{def: d, state: "stopped", log: newRing()}
	if err := m.save(); err != nil {
		delete(m.entries, d.ID)
		return err
	}
	return nil
}
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entries[id]
	if e == nil {
		return os.ErrNotExist
	}
	if e.proc != nil {
		return errors.New("instance is active")
	}
	delete(m.entries, id)
	if err := m.save(); err != nil {
		m.entries[id] = e
		return err
	}
	return nil
}
func view(e *entry) Instance { return Instance{Definition: e.def, State: e.state, Error: e.err} }
func (m *Manager) List() []Instance {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Instance, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, view(e))
	}
	slices.SortFunc(out, func(a, b Instance) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return out
}
func (m *Manager) Get(id string) (Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.entries[id]; e != nil {
		return view(e), nil
	}
	return Instance{}, os.ErrNotExist
}
func (m *Manager) Logs(id string) (string, error) {
	m.mu.Lock()
	e := m.entries[id]
	m.mu.Unlock()
	if e == nil {
		return "", os.ErrNotExist
	}
	return e.log.String(), nil
}
func (m *Manager) Start(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("manager closed")
	}
	e := m.entries[id]
	if e == nil {
		return os.ErrNotExist
	}
	if e.proc != nil {
		return errors.New("instance is active")
	}
	if err := m.validate(e.def, id); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	args := append(slices.Clone(e.def.Args), "-port", fmt.Sprint(e.def.Port))
	cmd := exec.CommandContext(ctx, e.def.Executable, args...)
	cmd.Dir = e.def.WorkDir
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second
	cmd.Stdout = e.log
	cmd.Stderr = e.log
	if err := cmd.Start(); err != nil {
		cancel()
		e.state = "failed"
		e.err = err.Error()
		return err
	}
	p := &process{cmd: cmd, cancel: cancel, done: make(chan struct{})}
	e.proc = p
	e.state = "running"
	e.err = ""
	go m.wait(id, p)
	return nil
}
func (m *Manager) wait(id string, p *process) {
	err := p.cmd.Wait()
	p.cancel()
	m.mu.Lock()
	e := m.entries[id]
	if e != nil && e.proc == p {
		e.proc = nil
		if e.state == "stopping" {
			e.state = "stopped"
			e.err = ""
		} else {
			e.state = "failed"
			if err != nil {
				e.err = err.Error()
			} else {
				e.err = "process exited"
			}
		}
	}
	close(p.done)
	m.mu.Unlock()
}
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	e := m.entries[id]
	if e == nil {
		m.mu.Unlock()
		return os.ErrNotExist
	}
	p := e.proc
	if p == nil {
		m.mu.Unlock()
		return errors.New("instance is not running")
	}
	e.state = "stopping"
	p.cancel()
	m.mu.Unlock()
	select {
	case <-p.done:
		return nil
	case <-time.After(10 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
		return nil
	}
}
func (m *Manager) Restart(id string) error {
	m.mu.Lock()
	e := m.entries[id]
	if e == nil {
		m.mu.Unlock()
		return os.ErrNotExist
	}
	active := e.proc != nil
	m.mu.Unlock()
	if active {
		if err := m.Stop(id); err != nil {
			return err
		}
	}
	return m.Start(id)
}
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	ids := make([]string, 0)
	for id, e := range m.entries {
		if e.proc != nil {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.Stop(id)
	}
}

const logLimit = 64 * 1024

type ring struct {
	mu sync.Mutex
	b  []byte
}

func newRing() *ring { return &ring{b: make([]byte, 0, logLimit)} }
func (r *ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(p)
	if n >= logLimit {
		r.b = append(r.b[:0], p[n-logLimit:]...)
		return n, nil
	}
	if len(r.b)+n > logLimit {
		r.b = append(r.b[:0], r.b[len(r.b)+n-logLimit:]...)
	}
	r.b = append(r.b, p...)
	return n, nil
}
func (r *ring) String() string { r.mu.Lock(); defer r.mu.Unlock(); return string(r.b) }

var _ io.Writer = (*ring)(nil)
