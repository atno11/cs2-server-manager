package tui

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/atno11/cs2-server-manager/internal/apiclient"
	"github.com/atno11/cs2-server-manager/internal/apitypes"
)

type eventStream interface {
	Next() (apiclient.Event, error)
	Close() error
}

type service interface {
	Subscribe(context.Context) (eventStream, error)
	Logs(context.Context, string) (apiclient.LogSnapshot, error)
	Register(context.Context, apitypes.Definition) (apitypes.Instance, error)
	Start(context.Context, string) (apitypes.Instance, error)
	Stop(context.Context, string) (apitypes.Instance, error)
	Restart(context.Context, string) (apitypes.Instance, error)
	Remove(context.Context, string) error
}

// LiveService is the sole bridge from the TUI to the public API client.
type LiveService struct{ Client *apiclient.Client }

func (s LiveService) Subscribe(ctx context.Context) (eventStream, error) {
	return s.Client.Subscribe(ctx)
}
func (s LiveService) Logs(ctx context.Context, id string) (apiclient.LogSnapshot, error) {
	return s.Client.Logs(ctx, id)
}
func (s LiveService) Register(ctx context.Context, d apitypes.Definition) (apitypes.Instance, error) {
	return s.Client.Register(ctx, d)
}
func (s LiveService) Start(ctx context.Context, id string) (apitypes.Instance, error) {
	return s.Client.Start(ctx, id)
}
func (s LiveService) Stop(ctx context.Context, id string) (apitypes.Instance, error) {
	return s.Client.Stop(ctx, id)
}
func (s LiveService) Restart(ctx context.Context, id string) (apitypes.Instance, error) {
	return s.Client.Restart(ctx, id)
}
func (s LiveService) Remove(ctx context.Context, id string) error { return s.Client.Remove(ctx, id) }

type connection string

const (
	connecting   connection = "CONNECTING"
	connected    connection = "CONNECTED"
	reconnecting connection = "RECONNECTING"
	disconnected connection = "DISCONNECTED"
	authFailed   connection = "AUTH FAILED"
)

type subscribedMsg struct {
	gen    uint64
	stream eventStream
	err    error
}
type eventMsg struct {
	gen   uint64
	event apiclient.Event
	err   error
}
type retryMsg struct{ gen uint64 }
type logMsg struct {
	gen    uint64
	ticket apiclient.LogTicket
	log    apiclient.LogSnapshot
	err    error
}
type mutationMsg struct {
	kind, id string
	err      error
}

type Model struct {
	api           service
	ctx           context.Context
	cancel        context.CancelFunc
	stream        eventStream
	streamCancel  context.CancelFunc
	logCancel     context.CancelFunc
	gen           uint64
	status        connection
	backoff       int
	connectedAt   time.Time
	logRetries    int
	logTicket     apiclient.LogTicket
	state         *apiclient.State
	instances     []apitypes.Instance
	selected      string
	wantSelected  string
	filter        textinput.Model
	filtering     bool
	form          []textinput.Model
	formIndex     int
	modal         string
	modalID       string
	pending       string
	pendingID     string
	focus         int // 0 instances, 1 detail/log
	detail        bool
	help          bool
	follow        bool
	view          viewport.Model
	listTop       int
	nearestIndex  int
	width, height int
	dark          bool
	mono          bool
	message       string
	endpoint      string
	closed        bool
}

func New(parent context.Context, api service, endpoint string) *Model {
	ctx, cancel := context.WithCancel(parent)
	f := textinput.New()
	f.Prompt = "Filter: "
	f.CharLimit = 64
	m := &Model{api: api, ctx: ctx, cancel: cancel, endpoint: endpoint, status: connecting, state: apiclient.NewState(), filter: f, follow: true, view: viewport.New(), dark: true, nearestIndex: -1}
	m.view.SoftWrap = false
	return m
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.subscribe(), tea.RequestBackgroundColor)
}

func (m *Model) Close() {
	if m.closed {
		return
	}
	m.closed = true
	m.cancel()
	if m.logCancel != nil {
		m.logCancel()
	}
	if m.streamCancel != nil {
		m.streamCancel()
	}
	if m.stream != nil {
		_ = m.stream.Close()
		m.stream = nil
	}
}

func (m *Model) subscribe() tea.Cmd {
	m.gen++
	gen := m.gen
	ctx, cancel := context.WithCancel(m.ctx)
	m.streamCancel = cancel
	return func() tea.Msg {
		s, err := m.api.Subscribe(ctx)
		return subscribedMsg{gen: gen, stream: s, err: err}
	}
}

func readEvent(gen uint64, s eventStream) tea.Cmd {
	return func() tea.Msg { e, err := s.Next(); return eventMsg{gen: gen, event: e, err: err} }
}

func (m *Model) reconnect(err error) tea.Cmd {
	stable := !m.connectedAt.IsZero() && time.Since(m.connectedAt) >= 30*time.Second
	m.connectedAt = time.Time{}
	if m.logCancel != nil {
		m.logCancel()
		m.logCancel = nil
	}
	if m.streamCancel != nil {
		m.streamCancel()
		m.streamCancel = nil
	}
	if m.stream != nil {
		_ = m.stream.Close()
		m.stream = nil
	}
	m.gen++ // invalidate outstanding command results
	m.message = "Stream interrupted; state is stale"
	var status *apiclient.StatusError
	if errors.As(err, &status) && status.Code == http.StatusUnauthorized {
		m.status = authFailed
		m.message = "Authentication failed; press R to retry or restart after updating the token file"
		return nil
	}
	m.status = disconnected
	if stable {
		m.backoff = 0
	}
	steps := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second}
	delay := steps[min(m.backoff, len(steps)-1)]
	if m.backoff < len(steps)-1 {
		m.backoff++
	}
	gen := m.gen
	ctx := m.ctx
	return func() tea.Msg {
		t := time.NewTimer(delay)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			return retryMsg{gen: gen}
		}
	}
}

func (m *Model) beginLog(id string) tea.Cmd {
	if m.logCancel != nil {
		m.logCancel()
	}
	m.view.SetContent("Recovering retained logs…")
	ticket, err := m.state.BeginLog(id)
	if err != nil {
		m.message = "Selected instance is unavailable"
		return nil
	}
	m.logTicket = ticket
	ctx, cancel := context.WithCancel(m.ctx)
	m.logCancel = cancel
	gen := m.gen
	return func() tea.Msg {
		log, err := m.api.Logs(ctx, id)
		return logMsg{gen: gen, ticket: ticket, log: log, err: err}
	}
}

func (m *Model) syncInstances() tea.Cmd {
	m.instances = m.state.Instances()
	if m.wantSelected != "" {
		for _, i := range m.instances {
			if i.ID == m.wantSelected {
				m.nearestIndex = -1
				m.selected = i.ID
				m.wantSelected = ""
				m.logRetries = 0
				return m.beginLog(m.selected)
			}
		}
	}
	for _, i := range m.instances {
		if i.ID == m.selected {
			m.nearestIndex = -1
			return nil
		}
	}
	visible := m.visible()
	if len(visible) == 0 {
		m.nearestIndex = -1
		m.selected = ""
		if m.logCancel != nil {
			m.logCancel()
		}
		m.view.SetContent("")
		return nil
	}
	idx := m.nearestIndex
	if idx < 0 {
		idx = m.listTop
	}
	m.nearestIndex = -1
	m.selected = visible[min(idx, len(visible)-1)].ID
	m.logRetries = 0
	return m.beginLog(m.selected)
}

func (m *Model) visible() []apitypes.Instance {
	filter := strings.ToLower(m.filter.Value())
	if filter == "" {
		return m.instances
	}
	out := make([]apitypes.Instance, 0, len(m.instances))
	for _, i := range m.instances {
		if strings.Contains(strings.ToLower(i.ID), filter) {
			out = append(out, i)
		}
	}
	return out
}

func (m *Model) selectID(id string) tea.Cmd {
	if id == m.selected {
		return nil
	}
	m.selected = id
	m.follow = true
	m.logRetries = 0
	if id == "" {
		return nil
	}
	return m.beginLog(id)
}

func (m *Model) refreshLog() {
	log, ok := m.state.Log()
	if !ok {
		m.view.SetContent("Recovering retained logs…")
		return
	}
	content := safeText(log.Bytes)
	if content == "" {
		content = "(no retained logs)"
	}
	m.view.SetContent(content)
	if m.follow {
		m.view.GotoBottom()
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
		m.resizeViewport()
		return m, nil
	case tea.BackgroundColorMsg:
		m.dark = v.IsDark()
		return m, nil
	case subscribedMsg:
		if v.gen != m.gen || m.closed {
			if v.stream != nil {
				_ = v.stream.Close()
			}
			return m, nil
		}
		if v.err != nil {
			return m, m.reconnect(v.err)
		}
		m.stream = v.stream
		return m, readEvent(m.gen, m.stream)
	case eventMsg:
		if v.gen != m.gen || m.closed {
			return m, nil
		}
		if v.err != nil {
			return m, m.reconnect(v.err)
		}
		if m.status != connected && v.event.Type != "snapshot" {
			return m, m.reconnect(apiclient.ErrInvalidEvent)
		}
		if v.event.Type == "snapshot" || v.event.Type == "instance.removed" {
			m.nearestIndex = slices.IndexFunc(m.visible(), func(i apitypes.Instance) bool { return i.ID == m.selected })
		}
		if err := m.state.Apply(v.event); err != nil {
			if errors.Is(err, apiclient.ErrLogGap) || errors.Is(err, apiclient.ErrLogBacklog) {
				if m.logRetries >= 3 {
					m.message = "Log recovery paused; press L to retry"
					return m, readEvent(m.gen, m.stream)
				}
				m.logRetries++
				return m, tea.Batch(m.beginLog(m.selected), readEvent(m.gen, m.stream))
			}
			if errors.Is(err, apiclient.ErrStaleLog) && m.selected != "" {
				if m.logRetries >= 3 {
					m.message = "Log recovery paused; press L to retry"
					return m, readEvent(m.gen, m.stream)
				}
				m.logRetries++
				return m, tea.Batch(m.beginLog(m.selected), readEvent(m.gen, m.stream))
			}
			return m, m.reconnect(err)
		}
		var cmd tea.Cmd
		if v.event.Type == "snapshot" {
			m.status = connected
			m.connectedAt = time.Now()
			m.logRetries = 0
			m.message = "Authoritative snapshot received"
			cmd = m.syncInstances() // BeginLog happens before scheduling the next read.
			if cmd == nil && m.selected != "" {
				cmd = m.beginLog(m.selected)
			}
		} else if v.event.Type == "instance.registered" || v.event.Type == "instance.removed" {
			cmd = m.syncInstances()
			if v.event.Type == "instance.registered" && v.event.Instance != nil && v.event.Instance.ID == m.selected && cmd == nil {
				cmd = m.beginLog(m.selected)
			}
		} else if v.event.Type == "instance.state" {
			m.instances = m.state.Instances()
		}
		if v.event.Type == "instance.log" && v.event.Log != nil && v.event.Log.ID == m.selected {
			m.refreshLog()
		}
		return m, tea.Batch(cmd, readEvent(m.gen, m.stream))
	case retryMsg:
		if v.gen != m.gen || m.closed || m.status != disconnected {
			return m, nil
		}
		m.status = reconnecting
		return m, m.subscribe()
	case logMsg:
		if v.gen != m.gen || m.closed || v.ticket != m.logTicket || m.selected == "" {
			return m, nil
		}
		if v.err != nil {
			if m.selected != "" && m.logRetries < 3 {
				m.logRetries++
				return m, m.beginLog(m.selected)
			}
			m.message = "Log request failed; press L to retry"
			return m, nil
		}
		if err := m.state.CompleteLog(v.ticket, v.log); err != nil {
			if errors.Is(err, apiclient.ErrStaleLog) {
				return m, nil
			}
			if m.selected != "" && m.logRetries < 3 {
				m.logRetries++
				return m, m.beginLog(m.selected)
			}
			m.message = "Log recovery paused; press L to retry"
			return m, nil
		}
		m.logRetries = 0
		m.refreshLog()
		return m, nil
	case mutationMsg:
		if v.kind != m.pending || v.id != m.pendingID {
			return m, nil
		}
		m.pending = ""
		m.pendingID = ""
		if v.err != nil {
			var status *apiclient.StatusError
			if errors.As(v.err, &status) {
				m.message = "Request rejected (HTTP " + strconv.Itoa(status.Code) + "); correct the input or state"
			} else {
				m.message = "Network failure; operation outcome unknown. Check authoritative state"
			}
			return m, nil
		}
		if v.kind == "register" {
			m.wantSelected = v.id
			m.modal = ""
			m.message = "Registration accepted; awaiting SSE"
			return m, m.syncInstances()
		}
		m.message = "Request accepted; awaiting SSE"
		return m, nil
	case tea.KeyPressMsg:
		return m, m.key(v)
	case tea.PasteMsg:
		return m, m.formPaste(v)
	case tea.MouseClickMsg:
		return m, m.click(v)
	case tea.MouseWheelMsg:
		m.wheel(v)
		return m, nil
	}
	return m, nil
}

func (m *Model) selectedInstance() (apitypes.Instance, bool) {
	idx := slices.IndexFunc(m.instances, func(i apitypes.Instance) bool { return i.ID == m.selected })
	if idx < 0 {
		return apitypes.Instance{}, false
	}
	return m.instances[idx], true
}
