package manager

import (
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
	cmd       *exec.Cmd
	done      chan struct{}
	pgid      int
	stopping  bool
	forced    bool
	signalErr error
}
type entry struct {
	def            Definition
	state, err, op string
	proc           *process
	log            *ring
}
type Manager struct {
	mu        sync.Mutex
	entries   map[string]*entry
	path      string
	closed    bool
	closeDone chan struct{}
	opWG      sync.WaitGroup
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func New(dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	m := &Manager{entries: map[string]*entry{}, path: filepath.Join(dir, "instances.json"), closeDone: make(chan struct{})}
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
		m.entries[d.ID] = &entry{def: copyDefinition(d), state: "stopped", log: newRing()}
	}
	return m, nil
}
func copyDefinition(d Definition) Definition { d.Args = slices.Clone(d.Args); return d }

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
	m.entries[d.ID] = &entry{def: copyDefinition(d), state: "stopped", log: newRing()}
	if err := m.save(); err != nil {
		delete(m.entries, d.ID)
		return err
	}
	return nil
}
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("manager closed")
	}
	e := m.entries[id]
	if e == nil {
		return os.ErrNotExist
	}
	if e.proc != nil || e.op != "" {
		return errors.New("instance is active")
	}
	delete(m.entries, id)
	if err := m.save(); err != nil {
		m.entries[id] = e
		return err
	}
	return nil
}
func view(e *entry) Instance {
	return Instance{Definition: copyDefinition(e.def), State: e.state, Error: e.err}
}
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

const gracefulTimeout = 2 * time.Second
const forcedTimeout = 2 * time.Second

func (m *Manager) reserve(id, op string) (*entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, errors.New("manager closed")
	}
	e := m.entries[id]
	if e == nil {
		return nil, os.ErrNotExist
	}
	if e.op != "" {
		return nil, errors.New("instance operation in progress")
	}
	if op == "start" && e.proc != nil {
		return nil, errors.New("instance is active")
	}
	if op == "stop" && e.proc == nil {
		return nil, errors.New("instance is not running")
	}
	e.op = op
	m.opWG.Add(1)
	return e, nil
}
func (m *Manager) release(e *entry) {
	m.mu.Lock()
	e.op = ""
	m.mu.Unlock()
	m.opWG.Done()
}
func (m *Manager) Start(id string) error {
	e, err := m.reserve(id, "start")
	if err != nil {
		return err
	}
	defer m.release(e)
	return m.startReserved(e)
}
func (m *Manager) startReserved(e *entry) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("manager closed")
	}
	if e.proc != nil {
		m.mu.Unlock()
		return errors.New("instance is active")
	}
	if err := m.validate(e.def, e.def.ID); err != nil {
		m.mu.Unlock()
		return err
	}
	d := copyDefinition(e.def)
	m.mu.Unlock()
	args := append(d.Args, "-port", fmt.Sprint(d.Port))
	cmd := exec.Command(d.Executable, args...)
	cmd.Dir = d.WorkDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = time.Second
	cmd.Stdout, cmd.Stderr = e.log, e.log
	if err := cmd.Start(); err != nil {
		m.mu.Lock()
		e.state = "failed"
		e.err = err.Error()
		m.mu.Unlock()
		return err
	}
	p := &process{cmd: cmd, done: make(chan struct{}), pgid: cmd.Process.Pid}
	m.mu.Lock()
	e.proc = p
	e.state = "running"
	e.err = ""
	shuttingDown := m.closed
	m.mu.Unlock()
	go m.wait(e, p)
	if shuttingDown {
		_ = m.terminate(e, p)
		return errors.New("manager closed")
	}
	return nil
}
func (m *Manager) wait(e *entry, p *process) {
	err := p.cmd.Wait()
	// The owned group may still contain children holding resources after its leader exits.
	cleanupErr := signalGroup(p.pgid, syscall.SIGKILL)
	m.mu.Lock()
	if e.proc == p {
		e.proc = nil
		switch {
		case p.forced:
			e.state = "failed"
			e.err = "forced termination after graceful timeout"
		case p.signalErr != nil:
			e.state = "failed"
			e.err = p.signalErr.Error()
		case p.stopping:
			e.state = "stopped"
			e.err = ""
		case err == nil || (errors.Is(err, exec.ErrWaitDelay) && p.cmd.ProcessState != nil && p.cmd.ProcessState.Success()):
			e.state = "stopped"
			e.err = ""
		default:
			e.state = "failed"
			e.err = err.Error()
		}
		if cleanupErr != nil {
			e.state = "failed"
			e.err = "process-group cleanup: " + cleanupErr.Error()
		}
	}
	close(p.done)
	m.mu.Unlock()
}
func signalGroup(pgid int, sig syscall.Signal) error {
	if pgid <= 0 {
		return errors.New("invalid process group")
	}
	err := syscall.Kill(-pgid, sig)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
func (m *Manager) terminate(e *entry, p *process) error {
	m.mu.Lock()
	p.stopping = true
	e.state = "stopping"
	m.mu.Unlock()
	if err := signalGroup(p.pgid, syscall.SIGTERM); err != nil {
		m.mu.Lock()
		p.signalErr = err
		m.mu.Unlock()
	}
	timer := time.NewTimer(gracefulTimeout)
	defer timer.Stop()
	select {
	case <-p.done:
	case <-timer.C:
		m.mu.Lock()
		p.forced = true
		m.mu.Unlock()
		if err := signalGroup(p.pgid, syscall.SIGKILL); err != nil {
			m.mu.Lock()
			p.signalErr = err
			m.mu.Unlock()
		}
		killTimer := time.NewTimer(forcedTimeout)
		defer killTimer.Stop()
		select {
		case <-p.done:
		case <-killTimer.C:
			m.mu.Lock()
			e.state = "failed"
			e.err = "process did not exit after forced termination"
			m.mu.Unlock()
			return errors.New("process termination timed out")
		}
	}
	m.mu.Lock()
	state, detail := e.state, e.err
	m.mu.Unlock()
	if state == "failed" {
		return errors.New(detail)
	}
	return nil
}
func (m *Manager) Stop(id string) error {
	e, err := m.reserve(id, "stop")
	if err != nil {
		return err
	}
	defer m.release(e)
	m.mu.Lock()
	p := e.proc
	m.mu.Unlock()
	if p == nil {
		return errors.New("instance is not running")
	}
	return m.terminate(e, p)
}
func (m *Manager) Restart(id string) error {
	e, err := m.reserve(id, "restart")
	if err != nil {
		return err
	}
	defer m.release(e)
	m.mu.Lock()
	p := e.proc
	m.mu.Unlock()
	if p != nil {
		if err := m.terminate(e, p); err != nil {
			return err
		}
	}
	return m.startReserved(e)
}
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		done := m.closeDone
		m.mu.Unlock()
		<-done
		return
	}
	m.closed = true
	m.mu.Unlock()
	m.opWG.Wait()
	m.mu.Lock()
	active := make([]struct {
		e *entry
		p *process
	}, 0)
	for _, e := range m.entries {
		if e.proc != nil {
			active = append(active, struct {
				e *entry
				p *process
			}{e, e.proc})
		}
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, item := range active {
		wg.Add(1)
		go func(e *entry, p *process) { defer wg.Done(); _ = m.terminate(e, p) }(item.e, item.p)
	}
	wg.Wait()
	close(m.closeDone)
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
