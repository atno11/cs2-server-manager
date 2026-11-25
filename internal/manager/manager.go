package manager

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
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
	State           string `json:"state"`
	Error           string `json:"error,omitempty"`
	LastTermination string `json:"last_termination,omitempty"`
}
type process struct {
	cmd       *exec.Cmd
	done      chan struct{}
	pgid      int
	pidfd     int
	exited    bool
	termAt    time.Time
	killSent  bool
	stopping  bool
	forced    bool
	signalErr error
}
type entry struct {
	def             Definition
	state, err, op  string
	lastTermination string
	proc            *process
	log             *ring
	workInfo        os.FileInfo
}
type Manager struct {
	mu        sync.Mutex
	entries   map[string]*entry
	path      string
	closed    bool
	closeDone chan struct{}
	opWG      sync.WaitGroup
	closeErr  error
	policy    trustPolicy
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func New(dir string, config Policy) (*Manager, error) {
	return newWithPreflight(dir, config, preflightLinux)
}
func newWithPreflight(dir string, config Policy, preflight func() error) (*Manager, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := secureDir(dir); err != nil {
		return nil, err
	}
	policy, err := compilePolicy(config)
	if err != nil {
		return nil, err
	}
	if err := preflight(); err != nil {
		return nil, fmt.Errorf("Linux supervision unavailable: %w", err)
	}
	m := &Manager{entries: map[string]*entry{}, path: filepath.Join(dir, "instances.json"), closeDone: make(chan struct{}), policy: policy}
	if _, err := os.Lstat(m.path); err == nil {
		if err := secureFile(m.path); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := os.Open(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 512*1024+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 512*1024 {
		return nil, errors.New("stored definitions exceed 512 KiB")
	}
	var rawDefs []json.RawMessage
	if err = DecodeUniqueJSON(b, &rawDefs); err != nil {
		return nil, err
	}
	if len(rawDefs) > 32 {
		return nil, errors.New("stored instance count exceeds 32")
	}
	for _, raw := range rawDefs {
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err != nil {
			return nil, err
		}
		if compact.Len() > 16*1024 {
			return nil, errors.New("stored definition exceeds 16 KiB")
		}
		var d Definition
		if err = DecodeUniqueJSON(raw, &d); err != nil {
			return nil, err
		}
		if err = m.validate(d, ""); err != nil {
			return nil, fmt.Errorf("stored %s: %w", d.ID, err)
		}
		workInfo, _ := os.Stat(d.WorkDir)
		m.entries[d.ID] = &entry{def: copyDefinition(d), state: "stopped", log: newRing(m.policy.secrets...), workInfo: workInfo}
	}
	return m, nil
}
func copyDefinition(d Definition) Definition { d.Args = slices.Clone(d.Args); return d }

func (m *Manager) validate(d Definition, except string) error {
	if !idPattern.MatchString(d.ID) {
		return errors.New("invalid ID")
	}
	for _, secret := range m.policy.secrets {
		if strings.Contains(d.ID, secret) || strings.Contains(fmt.Sprint(d.Port), secret) {
			return errors.New("public instance value contains a known secret")
		}
	}
	if d.Port < 1 || d.Port > 65535 {
		return errors.New("invalid port")
	}
	if !filepath.IsAbs(d.Executable) || !filepath.IsAbs(d.WorkDir) {
		return errors.New("paths must be absolute")
	}
	if len(d.Executable) > 4096 || len(d.WorkDir) > 4096 {
		return errors.New("path too long")
	}
	if _, err := m.policy.validateDefinition(d); err != nil {
		return err
	}
	fi, err := os.Stat(d.Executable)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0111 == 0 {
		return errors.New("executable must be an existing executable file")
	}
	fi, err = os.Stat(d.WorkDir)
	if err != nil || !fi.IsDir() {
		return errors.New("working directory must exist")
	}
	if len(d.Args) > 32 {
		return errors.New("too many arguments")
	}
	total := 0
	for _, a := range d.Args {
		total += len(a)
		if len(a) == 0 || len(a) > 256 || containsNUL(a) || unsafeArgument(a) || strings.HasPrefix(a, "-") || strings.HasPrefix(a, "+") {
			return errors.New("invalid launch argument")
		}
	}
	if total > 4096 {
		return errors.New("arguments too large")
	}

	for id, e := range m.entries {
		if id != except && (id == d.ID || e.def.Port == d.Port || sameDirectory(e.def.WorkDir, d.WorkDir)) {
			return errors.New("duplicate ID, port, or working directory")
		}
	}
	return nil
}
func sameDirectory(a, b string) bool {
	if a == b {
		return true
	}
	ai, ae := os.Stat(a)
	bi, be := os.Stat(b)
	return ae == nil && be == nil && os.SameFile(ai, bi)
}
func unsafeArgument(a string) bool {
	for _, r := range a {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '.' || r == '/' || r == '-') {
			return true
		}
	}
	return false
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
	if len(m.entries) >= 32 {
		return errors.New("instance limit reached")
	}
	if err := m.validate(d, ""); err != nil {
		return err
	}
	workInfo, _ := os.Stat(d.WorkDir)
	m.entries[d.ID] = &entry{def: copyDefinition(d), state: "stopped", log: newRing(m.policy.secrets...), workInfo: workInfo}
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
	return Instance{Definition: copyDefinition(e.def), State: e.state, Error: e.err, LastTermination: e.lastTermination}
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
	currentWork, err := os.Stat(e.def.WorkDir)
	if err != nil || !os.SameFile(e.workInfo, currentWork) {
		m.mu.Unlock()
		return errors.New("working directory identity changed")
	}
	d := copyDefinition(e.def)
	m.mu.Unlock()
	if err := preflightLinux(); err != nil {
		return fmt.Errorf("Linux supervision unavailable: %w", err)
	}
	execFile, err := m.policy.openExecutable(d.Executable)
	if err != nil {
		return err
	}
	defer execFile.Close()
	if _, err = execFile.Seek(0, 0); err != nil {
		return err
	}
	args := append(slices.Clone(d.Args), m.policy.baseArgs...)
	args = append(args, "-port", fmt.Sprint(d.Port))
	cmd := exec.Command("/proc/self/fd/3", args...)
	cmd.Args[0] = d.Executable
	cmd.ExtraFiles = []*os.File{execFile}
	cmd.Dir = d.WorkDir
	pidfd := -1
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, PidFD: &pidfd}
	cmd.WaitDelay = time.Second
	cmd.Stdout, cmd.Stderr = e.log, e.log
	if err := cmd.Start(); err != nil {
		m.mu.Lock()
		e.state = "failed"
		e.err = err.Error()
		m.mu.Unlock()
		return err
	}
	if pidfd < 0 {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return errors.New("Linux pidfd unavailable")
	}
	if err := pidfdSignalGroup(pidfd, 0); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = syscall.Close(pidfd)
		return fmt.Errorf("pidfd process-group signaling unavailable: %w", err)
	}
	p := &process{cmd: cmd, done: make(chan struct{}), pgid: cmd.Process.Pid, pidfd: pidfd}
	m.mu.Lock()
	e.proc = p
	e.state = "running"
	e.err = ""
	shuttingDown := m.closed
	m.mu.Unlock()
	go m.wait(e, p)
	if shuttingDown {
		result := m.terminate(e, p)
		return errors.Join(errors.New("manager closed"), result.err)
	}
	return nil
}

// waitLeader observes exit without reaping. The zombie leader anchors its group ID
// until all owned group signaling is finished and cmd.Wait reaps it.
func waitLeader(pid int) error {
	var info [128]byte
	for {
		_, _, errno := syscall.Syscall6(syscall.SYS_WAITID, 1, uintptr(pid), uintptr(unsafe.Pointer(&info[0])), uintptr(syscall.WEXITED|syscall.WNOWAIT), 0, 0)
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			return errno
		}
		return nil
	}
}

const pidfdSignalProcessGroup = 4
const sysPidfdSendSignal = 424 // Linux syscall number on supported Linux architectures.
func pidfdSignalGroup(fd int, sig syscall.Signal) error {
	if fd < 0 {
		return errors.New("invalid pidfd")
	}
	_, _, errno := syscall.Syscall6(sysPidfdSendSignal, uintptr(fd), uintptr(sig), 0, pidfdSignalProcessGroup, 0, 0)
	if errno == syscall.ESRCH {
		return nil
	}
	if errno != 0 {
		return errno
	}
	return nil
}

// groupHasLiveMembers excludes the unreaped leader and zombie descendants.
func groupHasLiveMembers(pgid, leader int) (bool, error) {
	dirs, err := os.ReadDir("/proc")
	if err != nil {
		return false, err
	}
	return scanGroupMembers(dirs, os.ReadFile, pgid, leader)
}
func scanGroupMembers(dirs []os.DirEntry, readStat func(string) ([]byte, error), pgid, leader int) (bool, error) {
	for _, d := range dirs {
		pid, err := strconv.Atoi(d.Name())
		if err != nil || pid == leader {
			continue
		}
		b, err := readStat("/proc/" + d.Name() + "/stat")
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
			continue
		}
		if err != nil {
			return false, err
		}
		rest := string(b)
		i := strings.LastIndex(rest, ") ")
		if i < 0 {
			continue
		}
		fields := strings.Fields(rest[i+2:])
		if len(fields) < 3 {
			continue
		}
		group, err := strconv.Atoi(fields[2])
		if err == nil && group == pgid && fields[0] != "Z" && fields[0] != "X" {
			return true, nil
		}
	}
	return false, nil
}
func (m *Manager) wait(e *entry, p *process) {
	observedErr := waitLeader(p.cmd.Process.Pid)
	m.mu.Lock()
	p.exited = true
	m.mu.Unlock()
	cleanupErr := observedErr
	if observedErr == nil {
		cleanupErr = m.cleanupGroup(e, p)
	}
	err := p.cmd.Wait()
	e.log.Flush()
	_ = syscall.Close(p.pidfd)
	m.mu.Lock()
	if e.proc == p {
		if cleanupErr == nil {
			e.proc = nil
		}
		switch {
		case cleanupErr != nil:
			e.state = "failed"
			e.err = "process-group cleanup: " + cleanupErr.Error()
		case p.signalErr != nil:
			e.state = "failed"
			e.err = p.signalErr.Error()
		case p.forced:
			e.state = "failed"
			e.err = "forced termination after graceful timeout"
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
		if p.forced {
			e.lastTermination = "forced termination after graceful timeout"
		}
		if cleanupErr != nil {
			e.lastTermination = "process-group cleanup: " + cleanupErr.Error()
		}
	}
	close(p.done)
	m.mu.Unlock()
}
func (m *Manager) cleanupGroup(e *entry, p *process) error {
	live, err := groupHasLiveMembers(p.pgid, p.cmd.Process.Pid)
	if err != nil {
		return err
	}
	if !live {
		return nil
	}
	m.mu.Lock()
	if p.termAt.IsZero() {
		p.termAt = time.Now()
		err = pidfdSignalGroup(p.pidfd, syscall.SIGTERM)
		if err != nil {
			p.signalErr = err
		}
	}
	deadline := p.termAt.Add(gracefulTimeout)
	m.mu.Unlock()
	if err != nil {
		return err
	}
	for time.Now().Before(deadline) {
		live, err = groupHasLiveMembers(p.pgid, p.cmd.Process.Pid)
		if err != nil {
			return err
		}
		if !live {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	m.mu.Lock()
	if !p.killSent {
		p.killSent = true
		p.forced = true
		err = pidfdSignalGroup(p.pidfd, syscall.SIGKILL)
		if err != nil {
			p.signalErr = err
		}
	}
	m.mu.Unlock()
	if err != nil {
		return err
	}
	deadline = time.Now().Add(forcedTimeout)
	for time.Now().Before(deadline) {
		live, err = groupHasLiveMembers(p.pgid, p.cmd.Process.Pid)
		if err != nil {
			return err
		}
		if !live {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("owned process group remained active after SIGKILL")
}

type terminationResult struct {
	confirmed, forced, replaceable bool
	err                            error
}

func (m *Manager) terminate(e *entry, p *process) terminationResult {
	m.mu.Lock()
	if e.proc != p {
		if e.proc == nil {
			result := terminationResult{confirmed: true}
			if e.state == "failed" {
				result.err = errors.New(e.err)
			}
			m.mu.Unlock()
			return result
		}
		m.mu.Unlock()
		return terminationResult{err: errors.New("stale process ownership")}
	}
	if !p.exited {
		p.stopping = true
		e.state = "stopping"
		if p.termAt.IsZero() {
			p.termAt = time.Now()
			if err := pidfdSignalGroup(p.pidfd, syscall.SIGTERM); err != nil {
				p.signalErr = err
			}
		}
	}
	termAt := p.termAt
	m.mu.Unlock()
	if termAt.IsZero() { // Natural exit already observed; waiter owns descendant cleanup.
		select {
		case <-p.done:
			return m.terminationResult(e, p)
		case <-time.After(gracefulTimeout + forcedTimeout + time.Second):
			return terminationResult{err: errors.New("process completion timed out")}
		}
	}
	timer := time.NewTimer(time.Until(termAt.Add(gracefulTimeout)))
	defer timer.Stop()
	select {
	case <-p.done:
		return m.terminationResult(e, p)
	case <-timer.C:
	}
	m.mu.Lock()
	if e.proc == p && !p.exited && !p.killSent {
		p.killSent = true
		p.forced = true
		if err := pidfdSignalGroup(p.pidfd, syscall.SIGKILL); err != nil {
			p.signalErr = err
		}
	}
	m.mu.Unlock()
	killTimer := time.NewTimer(forcedTimeout + time.Second)
	defer killTimer.Stop()
	select {
	case <-p.done:
		return m.terminationResult(e, p)
	case <-killTimer.C:
		m.mu.Lock()
		if e.proc == p {
			e.state = "failed"
			e.err = "process did not exit after forced termination"
		}
		m.mu.Unlock()
		return terminationResult{err: errors.New("process termination timed out")}
	}
}
func (m *Manager) terminationResult(e *entry, p *process) terminationResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.proc == p {
		return terminationResult{err: errors.New("process completion unconfirmed")}
	}
	if e.state == "failed" {
		return terminationResult{confirmed: true, forced: p.forced, replaceable: p.forced && p.signalErr == nil && e.err == "forced termination after graceful timeout", err: errors.New(e.err)}
	}
	return terminationResult{confirmed: true, forced: p.forced}
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
	return m.terminate(e, p).err
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
		result := m.terminate(e, p)
		if !result.confirmed || (result.err != nil && !result.replaceable) {
			return result.err
		}
	}
	return m.startReserved(e)
}
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		done := m.closeDone
		m.mu.Unlock()
		<-done
		m.mu.Lock()
		err := m.closeErr
		m.mu.Unlock()
		return err
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
	errs := make([]error, len(active))
	var wg sync.WaitGroup
	for i, item := range active {
		wg.Add(1)
		go func(i int, e *entry, p *process) {
			defer wg.Done()
			result := m.terminate(e, p)
			if result.err != nil {
				errs[i] = fmt.Errorf("%s: %w", e.def.ID, result.err)
			}
		}(i, item.e, item.p)
	}
	wg.Wait()
	m.mu.Lock()
	m.closeErr = errors.Join(errs...)
	close(m.closeDone)
	err := m.closeErr
	m.mu.Unlock()
	return err
}

const logLimit = 64 * 1024

type ring struct {
	mu        sync.Mutex
	b         []byte
	pending   []byte
	secrets   [][]byte
	longest   int
	covered   int
	redacting bool
}

func newRing(secrets ...string) *ring {
	r := &ring{b: make([]byte, 0, logLimit)}
	for _, s := range secrets {
		if len(s) < 4 {
			continue
		}
		v := []byte(s)
		r.secrets = append(r.secrets, v)
		if len(v) > r.longest {
			r.longest = len(v)
		}
	}
	return r
}
func (r *ring) appendBounded(p []byte) {
	if len(p) >= logLimit {
		r.b = append(r.b[:0], p[len(p)-logLimit:]...)
		return
	}
	if len(r.b)+len(p) > logLimit {
		r.b = append(r.b[:0], r.b[len(r.b)+len(p)-logLimit:]...)
	}
	r.b = append(r.b, p...)
}
func (r *ring) drain(all bool) {
	if r.longest == 0 {
		r.appendBounded(r.pending)
		r.pending = r.pending[:0]
		return
	}
	limit := len(r.pending) - r.longest + 1
	if all {
		limit = len(r.pending)
	}
	if limit < 0 {
		limit = 0
	}
	out := make([]byte, 0, limit)
	for i := 0; i < limit; i++ {
		for _, secret := range r.secrets {
			if len(r.pending)-i >= len(secret) && bytes.Equal(r.pending[i:i+len(secret)], secret) {
				if len(secret) > r.covered {
					r.covered = len(secret)
				}
			}
		}
		if r.covered > 0 {
			if !r.redacting {
				out = append(out, []byte("[REDACTED]")...)
				r.redacting = true
			}
			r.covered--
		} else {
			r.redacting = false
			out = append(out, r.pending[i])
		}
	}
	r.appendBounded(out)
	r.pending = append(r.pending[:0], r.pending[limit:]...)
}
func (r *ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(p)
	for len(p) > 0 {
		chunk := len(p)
		if chunk > 4096 {
			chunk = 4096
		}
		r.pending = append(r.pending, p[:chunk]...)
		r.drain(false)
		p = p[chunk:]
	}
	return n, nil
}
func (r *ring) Flush()         { r.mu.Lock(); defer r.mu.Unlock(); r.drain(true) }
func (r *ring) String() string { r.mu.Lock(); defer r.mu.Unlock(); return string(r.b) }

// RedactPublic removes known secret values from bounded HTTP response text.
func (m *Manager) RedactPublic(text string) string {
	r := newRing(m.policy.secrets...)
	_, _ = r.Write([]byte(text))
	r.Flush()
	return r.String()
}

var _ io.Writer = (*ring)(nil)
