package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atno11/cs2-server-manager/internal/apiclient"
	"github.com/atno11/cs2-server-manager/internal/apitypes"
)

type fakeStream struct {
	events chan apiclient.Event
	done   chan struct{}
}

func newFakeStream() *fakeStream {
	return &fakeStream{events: make(chan apiclient.Event, 16), done: make(chan struct{})}
}
func (s *fakeStream) Next() (apiclient.Event, error) {
	select {
	case e := <-s.events:
		return e, nil
	case <-s.done:
		return apiclient.Event{}, apiclient.ErrStreamInterrupted
	}
}
func (s *fakeStream) Close() error {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	return nil
}

type fakeAPI struct {
	stream    *fakeStream
	logs      apiclient.LogSnapshot
	logErr    error
	mutations []string
}

func (a *fakeAPI) Subscribe(context.Context) (eventStream, error) { return a.stream, nil }
func (a *fakeAPI) Logs(context.Context, string) (apiclient.LogSnapshot, error) {
	return a.logs, a.logErr
}
func (a *fakeAPI) Register(_ context.Context, d apitypes.Definition) (apitypes.Instance, error) {
	a.mutations = append(a.mutations, "register "+d.ID)
	return apitypes.Instance{ID: d.ID, State: "stopped", Port: d.Port}, nil
}
func (a *fakeAPI) Start(_ context.Context, id string) (apitypes.Instance, error) {
	a.mutations = append(a.mutations, "start "+id)
	return apitypes.Instance{}, nil
}
func (a *fakeAPI) Stop(_ context.Context, id string) (apitypes.Instance, error) {
	a.mutations = append(a.mutations, "stop "+id)
	return apitypes.Instance{}, nil
}
func (a *fakeAPI) Restart(_ context.Context, id string) (apitypes.Instance, error) {
	a.mutations = append(a.mutations, "restart "+id)
	return apitypes.Instance{}, nil
}
func (a *fakeAPI) Remove(_ context.Context, id string) error {
	a.mutations = append(a.mutations, "remove "+id)
	return nil
}

func testModel(t *testing.T) (*Model, *fakeAPI) {
	t.Helper()
	a := &fakeAPI{stream: newFakeStream(), logs: apiclient.LogSnapshot{Bytes: []byte("clean"), Start: 0, End: 5}}
	m := New(context.Background(), a, "http://127.0.0.1:8080")
	t.Cleanup(m.Close)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	return m, a
}

func TestRegistrationPaste(t *testing.T) {
	m, a := testModel(t)
	send(m, snapshot("existing"))
	m.Update(tea.PasteMsg{Content: "d"})
	if m.modal != "" || len(a.mutations) != 0 {
		t.Fatal("paste outside form triggered administrative action")
	}
	m.openForm()
	values := []string{"alpha", "/tmp/Sim é/exec", "/tmp/work dir/世界", "27015"}
	for i, value := range values {
		if i > 0 {
			m.formKey(tea.KeyPressMsg{Code: tea.KeyTab})
		}
		m.Update(tea.PasteMsg{Content: value})
		if got := m.form[i].Value(); got != value {
			t.Fatalf("field %d paste = %q, want %q", i, got, value)
		}
	}
	m.formKey(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	m.Update(tea.PasteMsg{Content: "ordered/arg"})
	if got := m.form[4].Value(); got != "ordered/arg" {
		t.Fatalf("argument paste = %q", got)
	}
	for _, unsafe := range []string{"x\ny", "x\ry", "x\x1by", "x\x00y", "x\ty", "x\u2028y"} {
		before := m.form[4].Value()
		m.Update(tea.PasteMsg{Content: unsafe})
		if m.form[4].Value() != before || m.pending != "" || len(a.mutations) != 0 {
			t.Fatalf("unsafe paste changed state: %q", unsafe)
		}
	}
	m.Update(tea.PasteMsg{Content: "\x1b[31my\nenter\x1b[0m"})
	if m.form[4].Value() != "ordered/arg" || m.modal != "form" || m.pending != "" {
		t.Fatal("paste triggered form action")
	}
}

func TestRegistrationShortcutThroughUpdate(t *testing.T) {
	press := tea.KeyPressMsg{Code: 'n', Text: "n"}
	for _, tc := range []struct {
		name      string
		prepare   func(*Model)
		wantModal string
	}{
		{"connected", func(m *Model) { send(m, snapshot()) }, "form"},
		{"after paste outside form", func(m *Model) { send(m, snapshot()); m.Update(tea.PasteMsg{Content: "n"}) }, "form"},
		{"connecting guard", func(*Model) {}, ""},
		{"pending guard", func(m *Model) { send(m, snapshot()); m.pending = "register" }, ""},
		{"filter context", func(m *Model) { send(m, snapshot()); m.Update(tea.KeyPressMsg{Code: '/', Text: "/"}) }, ""},
		{"help context", func(m *Model) { send(m, snapshot()); m.Update(tea.KeyPressMsg{Code: '?', Text: "?"}) }, ""},
		{"confirmation context", func(m *Model) { send(m, snapshot("one")); m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}) }, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, a := testModel(t)
			tc.prepare(m)
			m.Update(press)
			if m.modal != tc.wantModal || len(a.mutations) != 0 {
				t.Fatalf("modal=%q want=%q mutations=%v", m.modal, tc.wantModal, a.mutations)
			}
			if tc.name == "filter context" && m.filter.Value() != "n" {
				t.Fatalf("filter value = %q", m.filter.Value())
			}
		})
	}
}
func snapshot(ids ...string) apiclient.Event {
	v := make([]apitypes.Instance, 0, len(ids))
	for i, id := range ids {
		v = append(v, apitypes.Instance{ID: id, Port: 27015 + i, State: "stopped"})
	}
	return apiclient.Event{Type: "snapshot", Snapshot: &apitypes.Snapshot{Instances: v}}
}
func send(m *Model, e apiclient.Event) tea.Cmd {
	_, cmd := m.Update(eventMsg{gen: m.gen, event: e})
	return cmd
}

func TestModelSnapshotSelectionAndLogOrdering(t *testing.T) {
	m, _ := testModel(t)
	send(m, snapshot("a", "b"))
	if m.status != connected || m.selected != "a" {
		t.Fatalf("snapshot: %s %s", m.status, m.selected)
	}
	// The ticket is established synchronously before the next read command.
	if err := m.state.Apply(apiclient.Event{Type: "instance.log", Log: &apitypes.LogEvent{ID: "a", Start: 5, End: 6, Bytes: []byte("!")}}); err != nil {
		t.Fatal(err)
	}
	if m.logCancel == nil {
		t.Fatal("log recovery was not started before next read")
	}
	// A second snapshot replaces the view while preserving the stable ID.
	send(m, snapshot("b", "a"))
	if m.selected != "a" {
		t.Fatal("selection followed row position")
	}
	send(m, snapshot("b"))
	if m.selected != "b" {
		t.Fatal("selection did not recover after removal")
	}
	send(m, snapshot())
	if m.selected != "" {
		t.Fatal("empty snapshot retained invalid selection")
	}
}

func TestRemovedSelectionChoosesNearestVisibleRow(t *testing.T) {
	m, _ := testModel(t)
	send(m, snapshot("a", "b", "c", "d"))
	m.selectID("c")
	send(m, apiclient.Event{Type: "instance.removed", RemovedID: "c"})
	if m.selected != "d" {
		t.Fatalf("nearest row after removal: %q", m.selected)
	}
	send(m, snapshot("a", "b"))
	if m.selected != "b" {
		t.Fatalf("nearest row after snapshot: %q", m.selected)
	}
}

func TestModelInteractionAndConfirmation(t *testing.T) {
	m, a := testModel(t)
	send(m, snapshot("one", "two"))
	m.key(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if m.selected != "two" {
		t.Fatal("vim navigation")
	}
	m.key(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.modal != "stop" {
		t.Fatal("missing confirmation")
	}
	m.key(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.modal != "" || len(a.mutations) != 0 {
		t.Fatal("cancel mutated")
	}
	m.key(tea.KeyPressMsg{Code: 'x', Text: "x"})
	cmd := m.key(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("confirmation command missing")
	}
	cmd()
	if len(a.mutations) != 1 || a.mutations[0] != "stop two" {
		t.Fatalf("wrong mutation: %v", a.mutations)
	}
	if m.key(tea.KeyPressMsg{Code: 's', Text: "s"}) != nil {
		t.Fatal("duplicate mutation queued")
	}
	m.Update(mutationMsg{kind: "stop", id: "two"})
	m.key(tea.KeyPressMsg{Code: '/', Text: "/"})
	m.key(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.closed {
		t.Fatal("form/filter key quit app")
	}
}

func TestRegistrationValidationAndForm(t *testing.T) {
	m, a := testModel(t)
	send(m, snapshot())
	m.openForm()
	for i, v := range []string{"alpha", "/tmp/sim", "/tmp/work", "27015"} {
		m.form[i].SetValue(v)
	}
	d, err := m.definition()
	if err != nil || d.ID != "alpha" {
		t.Fatalf("valid form: %+v %v", d, err)
	}
	m.form[3].SetValue("0")
	if _, err = m.definition(); err == nil {
		t.Fatal("invalid port")
	}
	m.form[3].SetValue("27015")
	m.formKey(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if len(m.form) != 5 {
		t.Fatal("argument field not added")
	}
	m.form[4].SetValue("ordered/arg")
	if d, err := m.definition(); err != nil || len(d.Args) != 1 || d.Args[0] != "ordered/arg" {
		t.Fatal("ordered argument lost")
	}
	m.form[4].SetValue("-port")
	if _, err := m.definition(); err == nil {
		t.Fatal("option-like argument accepted")
	}
	m.form[4].SetValue("ordered/arg")
	m.formIndex = 4
	cmd := m.formKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("registration not submitted")
	}
	cmd()
	if len(a.mutations) != 1 || a.mutations[0] != "register alpha" {
		t.Fatal(a.mutations)
	}
	m.Update(mutationMsg{kind: "register", id: "alpha"})
	if m.modal != "" || m.pending != "" {
		t.Fatal("form not closed after accepted registration")
	}
}

func TestModelRenderingBoundsAndMouse(t *testing.T) {
	m, _ := testModel(t)
	send(m, snapshot("east", "west"))
	for _, size := range []tea.WindowSizeMsg{{Width: 120, Height: 24}, {Width: 80, Height: 20}, {Width: 60, Height: 18}, {Width: 40, Height: 10}} {
		m.Update(size)
		v := m.View().Content
		lines := strings.Split(v, "\n")
		if len(lines) != size.Height {
			t.Fatalf("height %d: %d", size.Height, len(lines))
		}
		for _, line := range lines {
			if lipgloss.Width(line) > size.Width {
				t.Fatalf("line exceeds %d cells", size.Width)
			}
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m.click(tea.MouseClickMsg{X: 3, Y: 5, Button: tea.MouseLeft})
	if m.selected != "west" {
		t.Fatalf("mouse row selection: %q", m.selected)
	}
	m.wheel(tea.MouseWheelMsg{X: 90, Y: 8, Button: tea.MouseWheelUp})
	if m.follow {
		t.Fatal("wheel did not pause follow")
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m.focus = 0
	m.click(tea.MouseClickMsg{X: 3, Y: 4, Button: tea.MouseLeft})
	if m.focus != 0 || m.selected != "east" {
		t.Fatal("compact list click changed panel or wrong selection")
	}
}

func TestSplitTableKeepsPortAndPanelBoundaries(t *testing.T) {
	m, _ := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	longID := strings.Repeat("a", 64)
	send(m, apiclient.Event{Type: "snapshot", Snapshot: &apitypes.Snapshot{Instances: []apitypes.Instance{{ID: longID, State: "stopped", Port: 65535}}}})
	lines := strings.Split(m.View().Content, "\n")
	if len(lines) != 20 {
		t.Fatalf("height: %d", len(lines))
	}
	if !strings.Contains(lines[3], "PORT") {
		t.Fatalf("PORT heading clipped: %q", lines[3])
	}
	if !strings.Contains(lines[4], "65535") || !strings.Contains(lines[4], "STOPPED") {
		t.Fatalf("state or port clipped: %q", lines[4])
	}
	if !strings.Contains(lines[4], "…") {
		t.Fatalf("long ID was not truncated first: %q", lines[4])
	}
	if !strings.Contains(lines[2], "INSTANCES") || !strings.Contains(lines[2], "LOGS") {
		t.Fatal("panel titles missing")
	}
	for _, line := range lines {
		if lipgloss.Width(line) != 100 {
			t.Fatalf("line is not exactly 100 cells: %d", lipgloss.Width(line))
		}
	}
	row := []rune(lines[4])
	if row[0] != '┃' || row[39] != '┃' || row[40] != ' ' || row[41] != '│' || row[99] != '│' {
		t.Fatalf("split panel boundaries: %q", lines[4])
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if !strings.Contains(m.View().Content, "65535") {
		t.Fatal("compact layout clipped port")
	}
	m.detail = true
	send(m, apiclient.Event{Type: "instance.state", Instance: &apitypes.Instance{ID: longID, State: "running", Port: 65535, Error: strings.Repeat("中", 60)}})
	for _, line := range strings.Split(m.View().Content, "\n") {
		if lipgloss.Width(line) > 80 {
			t.Fatal("wide Unicode overflow after resize")
		}
	}
}

func TestMouseUsesRenderedPanelBounds(t *testing.T) {
	m, _ := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	send(m, snapshot("alpha", "beta"))
	g := m.geometry()
	if !g.split || g.leftWidth != 40 || g.rightStart != 41 || g.rightWidth != 59 {
		t.Fatalf("geometry: %+v", g)
	}
	m.click(tea.MouseClickMsg{X: 40, Y: 4, Button: tea.MouseLeft})
	if m.focus != 0 || m.selected != "alpha" {
		t.Fatal("separator click changed focus or selection")
	}
	m.click(tea.MouseClickMsg{X: 39, Y: 5, Button: tea.MouseLeft})
	if m.focus != 0 || m.selected != "beta" {
		t.Fatal("last list cell did not select second row")
	}
	m.click(tea.MouseClickMsg{X: 41, Y: 4, Button: tea.MouseLeft})
	if m.focus != 1 {
		t.Fatal("first right-panel cell did not focus details")
	}
	m.click(tea.MouseClickMsg{X: g.detailToggleX, Y: g.top, Button: tea.MouseLeft})
	if !m.detail {
		t.Fatal("details title click missed")
	}
	m.click(tea.MouseClickMsg{X: g.detailToggleX - 1, Y: g.top, Button: tea.MouseLeft})
	if m.detail {
		t.Fatal("logs title click missed")
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if m.geometry().split {
		t.Fatal("compact threshold mismatch")
	}
}

func TestModelReconnectAndStaleGeneration(t *testing.T) {
	m, _ := testModel(t)
	send(m, snapshot("one"))
	old := m.gen
	_, cmd := m.Update(eventMsg{gen: old, err: errors.New("disconnect")})
	if m.status != disconnected || cmd == nil {
		t.Fatal("no bounded reconnect")
	}
	m.Update(eventMsg{gen: old, event: snapshot("stale")})
	if m.selected == "stale" {
		t.Fatal("old stream changed state")
	}
	m.Update(retryMsg{gen: m.gen})
	if m.status != reconnecting {
		t.Fatal("retry not started")
	}
	m.Update(subscribedMsg{gen: m.gen, err: &apiclient.StatusError{Code: 401}})
	if m.status != authFailed {
		t.Fatal("authentication failure retried")
	}
	if m.key(tea.KeyPressMsg{Code: 'R', Text: "R"}) == nil {
		t.Fatal("explicit reconnect unavailable")
	}
}

func TestSnapshotRecoversRetainedLogsForUnchangedSelection(t *testing.T) {
	for _, state := range []string{"stopped", "running"} {
		t.Run(state, func(t *testing.T) {
			m, a := testModel(t)
			a.logs = apiclient.LogSnapshot{Bytes: []byte("retained-" + state), End: uint64(len("retained-" + state))}
			e := snapshot("one")
			e.Snapshot.Instances[0].State = state
			_, first := m.Update(eventMsg{gen: m.gen, event: e})
			if first == nil {
				t.Fatal("initial snapshot did not schedule commands")
			}
			m.Update(m.beginLog("one")())
			if log, ok := m.state.Log(); !ok || string(log.Bytes) != "retained-"+state {
				t.Fatal("initial retained log missing")
			}
			_, next := m.Update(eventMsg{gen: m.gen, event: e})
			if err := m.state.Apply(apiclient.Event{Type: "instance.log", Log: &apitypes.LogEvent{ID: "one", Start: uint64(len("retained-" + state)), End: uint64(len("retained-"+state)) + 1, Bytes: []byte("!")}}); err != nil {
				t.Fatalf("ticket was not established before next event: %v", err)
			}
			batch, ok := next().(tea.BatchMsg)
			if !ok || len(batch) < 2 {
				t.Fatalf("snapshot did not schedule retained fetch before next read: %T, %d commands", batch, len(batch))
			}
			result := batch[0]()
			if _, ok := result.(logMsg); !ok {
				t.Fatalf("first command = %T, want log fetch", result)
			}
			m.Update(result)
			if log, ok := m.state.Log(); !ok || string(log.Bytes) != "retained-"+state+"!" {
				t.Fatal("reconnected retained log missing")
			}
		})
	}
}

func TestStaleFailedLogRequestDoesNotRetryCurrentSelection(t *testing.T) {
	for _, next := range []string{"other selection", "new attempt"} {
		t.Run(next, func(t *testing.T) {
			m, a := testModel(t)
			send(m, snapshot("one", "two"))
			a.logErr = errors.New("canceled request")
			old := m.beginLog("one")
			if next == "other selection" {
				m.selectID("two")
			} else {
				m.beginLog("one")
			}
			m.Update(old())
			if m.logRetries != 0 || m.message == "Log request failed; press L to retry" {
				t.Fatalf("stale failure changed recovery: retries=%d message=%q", m.logRetries, m.message)
			}
		})
	}
}

func TestStaleLogEventRecoveryIsBounded(t *testing.T) {
	m, _ := testModel(t)
	send(m, snapshot("one"))
	for i := 0; i < 4; i++ {
		if err := m.state.Apply(snapshot("one")); err != nil {
			t.Fatal(err)
		}
		m.Update(eventMsg{gen: m.gen, event: apiclient.Event{Type: "instance.log", Log: &apitypes.LogEvent{ID: "one", Start: 0, End: 1, Bytes: []byte("x")}}})
	}
	if m.logRetries != 3 || m.message != "Log recovery paused; press L to retry" {
		t.Fatalf("stale recovery unbounded: retries=%d message=%q", m.logRetries, m.message)
	}
}

func TestFailedSubscribeDoesNotReusePreviousConnectedAt(t *testing.T) {
	m, _ := testModel(t)
	send(m, snapshot())
	m.connectedAt = time.Now().Add(-time.Minute)
	m.reconnect(errors.New("stable stream ended"))
	if m.backoff != 1 {
		t.Fatalf("stable disconnect backoff=%d", m.backoff)
	}
	m.Update(subscribedMsg{gen: m.gen, err: errors.New("subscription failed")})
	if m.backoff != 2 {
		t.Fatalf("failed subscription reset backoff to %d", m.backoff)
	}
}

func TestReconnectBackoffResetsOnlyAfterStableConnection(t *testing.T) {
	m, _ := testModel(t)
	send(m, snapshot())
	m.reconnect(errors.New("short stream"))
	if m.backoff != 1 {
		t.Fatal("first retry")
	}
	send(m, snapshot())
	m.reconnect(errors.New("second short stream"))
	if m.backoff != 2 {
		t.Fatal("short-lived stream reset backoff")
	}
	m.connectedAt = time.Now().Add(-time.Minute)
	m.reconnect(errors.New("stable stream ended"))
	if m.backoff != 1 {
		t.Fatal("stable stream did not reset backoff")
	}
}

func TestSafePresentationRejectsTerminalCommands(t *testing.T) {
	raw := []byte("ok\x1b]52;c;AAAA\x07\x1b[2J\x1bPbad\x1b\\\r\b\x7f\xc2\x9b\xe2\x80\xae\xff\nend")
	s := safeText(raw)
	if strings.ContainsRune(s, '\x1b') || strings.ContainsRune(s, '\x07') || strings.Contains(s, "\u202e") || strings.Contains(s, "\ufffd") {
		t.Fatal("unsafe terminal content survived")
	}
	if !strings.Contains(s, "\\x1B") || !strings.Contains(s, "\\u202E") || !strings.Contains(s, "\\xFF") {
		t.Fatal("control bytes not visibly escaped")
	}
	m, _ := testModel(t)
	send(m, snapshot("one"))
	m.state.CompleteLog(func() apiclient.LogTicket { ticket, _ := m.state.BeginLog("one"); return ticket }(), apiclient.LogSnapshot{Bytes: raw, End: uint64(len(raw))})
	m.refreshLog()
	v := m.View().Content
	if strings.ContainsRune(v, '\x1b') && !strings.Contains(v, "\x1b[") {
		t.Fatal("untrusted escape reached view")
	}
	if strings.ContainsRune(m.view.GetContent(), '\x1b') || strings.Contains(m.view.GetContent(), "\u202e") {
		t.Fatal("untrusted log reached viewport")
	}
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	send(m, apiclient.Event{Type: "instance.state", Instance: &apitypes.Instance{ID: "one", Port: 27015, State: "unknown", Error: "\x1b]0;forged\x07"}})
	if strings.Contains(m.View().Content, "\x1b]0;forged") {
		t.Fatal("public error escaped presentation guard")
	}
}

func TestStaleLogResponseAfterSelection(t *testing.T) {
	m, _ := testModel(t)
	send(m, snapshot("a", "b"))
	old, err := m.state.BeginLog("a")
	if err != nil {
		t.Fatal(err)
	}
	m.selectID("b")
	m.Update(logMsg{gen: m.gen, ticket: old, log: apiclient.LogSnapshot{Bytes: []byte("old"), End: 3}})
	if log, ok := m.state.Log(); ok && log.ID == "a" {
		t.Fatal("stale selected log applied")
	}
}

func TestModelIncrementalLogReconciliation(t *testing.T) {
	m, _ := testModel(t)
	send(m, snapshot("a"))
	fetch := m.beginLog("a")
	send(m, apiclient.Event{Type: "instance.log", Log: &apitypes.LogEvent{ID: "a", Start: 5, End: 6, Bytes: []byte("!")}})
	m.Update(fetch())
	log, ok := m.state.Log()
	if !ok || log.ID != "a" || string(log.Bytes) != "clean!" || log.End != 6 {
		t.Fatalf("reconciliation: %+v", log)
	}
	// Overlap is suppressed by the client state helper, not by rendered string length.
	send(m, apiclient.Event{Type: "instance.log", Log: &apitypes.LogEvent{ID: "a", Start: 5, End: 6, Bytes: []byte("!")}})
	log, _ = m.state.Log()
	if string(log.Bytes) != "clean!" {
		t.Fatal("duplicate log bytes")
	}
}

func TestFilterThemeAndLogScroll(t *testing.T) {
	m, _ := testModel(t)
	send(m, snapshot("alpha", "beta"))
	m.filter.SetValue("bet")
	m.ensureVisibleSelection()
	if m.selected != "beta" || len(m.visible()) != 1 {
		t.Fatal("filter did not select visible ID")
	}
	m.filter.SetValue("")
	m.dark = false
	light := m.View().Content
	m.dark = true
	dark := m.View().Content
	m.mono = true
	mono := m.View().Content
	for _, v := range []string{light, dark, mono} {
		if !strings.Contains(v, "CONNECTED") || !strings.Contains(v, "beta") {
			t.Fatal("theme lost state labels")
		}
	}
	m.focus = 1
	m.view.SetContent(strings.Repeat("line\n", 80))
	m.view.GotoBottom()
	m.key(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if !m.view.AtTop() || m.follow {
		t.Fatal("top navigation did not pause follow")
	}
	m.key(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if !m.view.AtBottom() || !m.follow {
		t.Fatal("bottom navigation did not resume follow")
	}
	m.key(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if m.follow {
		t.Fatal("follow toggle")
	}
}

func TestSanitizerBoundaryAndExpansion(t *testing.T) {
	raw := append([]byte{0x9b, 0x1b, '[', 'H', '\n'}, []byte(strings.Repeat("a", 64*1024-7))...)
	raw = append(raw, 0xe2, 0x80)
	text := safeText(raw)
	if strings.ContainsRune(text, '\x1b') || strings.ContainsRune(text, '\ufffd') || !strings.Contains(text, "\\x9B") || !strings.HasSuffix(text, "\\xE2\\x80") {
		t.Fatal("retained boundary was unsafe")
	}
	if len(text) > 4*64*1024 {
		t.Fatal("presentation expanded beyond bounded factor")
	}
}

func TestLoadTokenRejectsUnsafeFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "admin.token")
	if _, err := LoadToken(path); err == nil {
		t.Fatal("missing token accepted")
	}
	token := strings.Repeat("a", 64)
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadToken(path)
	if err != nil || got != token {
		t.Fatalf("private token rejected: %v", err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(path); err == nil {
		t.Fatal("public token accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/null", path); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(path); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(path); err == nil {
		t.Fatal("invalid credential accepted")
	}
}

func TestCancellableStreamCleanup(t *testing.T) {
	m, a := testModel(t)
	_, cmd := m.Update(subscribedMsg{gen: m.gen, stream: a.stream})
	if cmd == nil {
		t.Fatal("read missing")
	}
	done := make(chan struct{})
	go func() { cmd(); close(done) }()
	m.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream read not released")
	}
}
