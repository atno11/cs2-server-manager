package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/atno11/cs2-server-manager/internal/apitypes"
)

var idRule = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var argRule = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

func (m *Model) key(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	if s == "ctrl+c" {
		m.Close()
		return tea.Quit
	}
	if m.modal == "form" {
		return m.formKey(k)
	}
	if m.modal != "" {
		switch s {
		case "esc", "n":
			m.modal = ""
			return nil
		case "y":
			kind, id := m.modal, m.modalID
			m.modal = ""
			return m.mutate(kind, id, apitypes.Definition{})
		}
		return nil
	}
	if m.filtering {
		if s == "esc" {
			m.filtering = false
			m.filter.Blur()
			m.filter.SetValue("")
			m.listTop = 0
			return m.ensureVisibleSelection()
		}
		if s == "enter" {
			m.filtering = false
			m.filter.Blur()
			return m.ensureVisibleSelection()
		}
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(k)
		m.listTop = 0
		return tea.Batch(cmd, m.ensureVisibleSelection())
	}
	if m.help {
		if s == "?" || s == "esc" {
			m.help = false
		}
		return nil
	}
	switch s {
	case "q":
		m.Close()
		return tea.Quit
	case "?":
		m.help = true
		return nil
	case "tab", "shift+tab":
		m.focus = 1 - m.focus
		return nil
	case "/":
		m.filtering = true
		return m.filter.Focus()
	case "n":
		if m.status == connected && m.pending == "" {
			m.openForm()
		}
		return nil
	case "R":
		if m.status == authFailed || m.status == disconnected || m.status == reconnecting {
			m.stopStream()
			m.status = reconnecting
			m.backoff = 0
			return m.subscribe()
		}
	case "L":
		if m.status == connected && m.selected != "" {
			m.logRetries = 0
			return m.beginLog(m.selected)
		}
	case "s":
		if m.status == connected && m.pending == "" && m.selected != "" {
			return m.mutate("start", m.selected, apitypes.Definition{})
		}
	case "x", "r", "d":
		if m.status == connected && m.pending == "" && m.selected != "" {
			m.modal = map[string]string{"x": "stop", "r": "restart", "d": "remove"}[s]
			m.modalID = m.selected
		}
		return nil
	case "v":
		m.detail = !m.detail
		return nil
	case "enter":
		if m.focus == 0 {
			m.focus = 1
		}
		return nil
	}
	if m.focus == 0 {
		visible := m.visible()
		idx := -1
		for i, v := range visible {
			if v.ID == m.selected {
				idx = i
				break
			}
		}
		switch s {
		case "up", "k":
			idx--
		case "down", "j":
			idx++
		default:
			return nil
		}
		if len(visible) == 0 {
			return nil
		}
		idx = max(0, min(idx, len(visible)-1))
		m.adjustListTop(idx)
		return m.selectID(visible[idx].ID)
	}
	if m.detail {
		return nil
	}
	switch s {
	case "up", "k":
		m.follow = false
		m.view.ScrollUp(1)
	case "down", "j":
		m.view.ScrollDown(1)
	case "pgup":
		m.follow = false
		m.view.PageUp()
	case "pgdown":
		m.view.PageDown()
	case "g":
		m.follow = false
		m.view.GotoTop()
	case "G":
		m.follow = true
		m.view.GotoBottom()
	case "f":
		m.follow = !m.follow
		if m.follow {
			m.view.GotoBottom()
		}
	}
	return nil
}

func (m *Model) stopStream() {
	if m.streamCancel != nil {
		m.streamCancel()
		m.streamCancel = nil
	}
	if m.stream != nil {
		_ = m.stream.Close()
		m.stream = nil
	}
	if m.logCancel != nil {
		m.logCancel()
		m.logCancel = nil
	}
	m.gen++
}

func (m *Model) ensureVisibleSelection() tea.Cmd {
	v := m.visible()
	if len(v) == 0 {
		return nil
	}
	for _, i := range v {
		if i.ID == m.selected {
			return nil
		}
	}
	return m.selectID(v[0].ID)
}

func (m *Model) adjustListTop(idx int) {
	rows := max(1, m.listRows())
	if idx < m.listTop {
		m.listTop = idx
	}
	if idx >= m.listTop+rows {
		m.listTop = idx - rows + 1
	}
}

func (m *Model) openForm() {
	m.modal = "form"
	m.formIndex = 0
	m.form = nil
	for _, p := range []string{"ID", "Executable", "Work dir", "Port"} {
		v := textinput.New()
		v.Prompt = p + ": "
		v.CharLimit = 4096
		v.SetWidth(max(12, m.width-18))
		m.form = append(m.form, v)
	}
	m.form[0].CharLimit = 64
	m.form[3].CharLimit = 5
	m.form[0].Focus()
}

func (m *Model) formKey(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	switch s {
	case "esc":
		m.modal = ""
		return nil
	case "ctrl+a":
		if len(m.form) < 36 {
			v := textinput.New()
			v.Prompt = fmt.Sprintf("Arg %d: ", len(m.form)-3)
			v.CharLimit = 256
			v.SetWidth(max(12, m.width-18))
			m.form = append(m.form, v)
			m.form[m.formIndex].Blur()
			m.formIndex = len(m.form) - 1
			return m.form[m.formIndex].Focus()
		}
		return nil
	case "ctrl+d":
		if m.formIndex >= 4 {
			m.form = append(m.form[:m.formIndex], m.form[m.formIndex+1:]...)
			m.formIndex = max(0, m.formIndex-1)
			return m.form[m.formIndex].Focus()
		}
		return nil
	case "tab", "shift+tab":
		m.form[m.formIndex].Blur()
		if s == "tab" {
			m.formIndex = (m.formIndex + 1) % len(m.form)
		} else {
			m.formIndex = (m.formIndex + len(m.form) - 1) % len(m.form)
		}
		return m.form[m.formIndex].Focus()
	case "enter":
		if m.formIndex < len(m.form)-1 {
			m.form[m.formIndex].Blur()
			m.formIndex++
			return m.form[m.formIndex].Focus()
		}
		def, err := m.definition()
		if err != nil {
			m.message = err.Error()
			return nil
		}
		return m.mutate("register", def.ID, def)
	}
	var cmd tea.Cmd
	m.form[m.formIndex], cmd = m.form[m.formIndex].Update(k)
	return cmd
}

func (m *Model) formPaste(p tea.PasteMsg) tea.Cmd {
	if m.modal != "form" || m.formIndex < 0 || m.formIndex >= len(m.form) {
		return nil
	}
	if !utf8.ValidString(p.Content) {
		m.message = "Paste rejected: invalid text"
		return nil
	}
	for _, r := range p.Content {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			m.message = "Paste rejected: control or multiline text"
			return nil
		}
	}
	var cmd tea.Cmd
	m.form[m.formIndex], cmd = m.form[m.formIndex].Update(p)
	return cmd
}

func (m *Model) definition() (apitypes.Definition, error) {
	if len(m.form) < 4 {
		return apitypes.Definition{}, errors.New("incomplete registration")
	}
	id, exe, work, portText := m.form[0].Value(), m.form[1].Value(), m.form[2].Value(), m.form[3].Value()
	if !idRule.MatchString(id) {
		return apitypes.Definition{}, errors.New("ID must use 1–64 ASCII letters, digits, _ or -")
	}
	if len(exe) == 0 || len(work) == 0 || len(exe) > 4096 || len(work) > 4096 || !filepath.IsAbs(exe) || !filepath.IsAbs(work) || filepath.Clean(exe) != exe || filepath.Clean(work) != work {
		return apitypes.Definition{}, errors.New("paths must be absolute, clean and at most 4096 bytes")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return apitypes.Definition{}, errors.New("port must be 1–65535")
	}
	d := apitypes.Definition{ID: id, Executable: exe, WorkDir: work, Port: port}
	bytes := 0
	for i := 4; i < len(m.form); i++ {
		a := m.form[i].Value()
		if len(a) == 0 || len(a) > 256 || !argRule.MatchString(a) || strings.HasPrefix(a, "-") {
			return apitypes.Definition{}, errors.New("arguments must be positional ASCII tokens, 1–256 bytes")
		}
		bytes += len(a)
		d.Args = append(d.Args, a)
	}
	if bytes > 4096 {
		return apitypes.Definition{}, errors.New("arguments exceed 4096 bytes")
	}
	return d, nil
}

func (m *Model) mutate(kind, id string, d apitypes.Definition) tea.Cmd {
	if m.pending != "" || m.status != connected {
		return nil
	}
	m.pending = kind
	m.pendingID = id
	m.message = "Pending " + kind + " for " + safeLine(id)
	ctx := m.ctx
	api := m.api
	return func() tea.Msg {
		var err error
		switch kind {
		case "register":
			_, err = api.Register(ctx, d)
		case "start":
			_, err = api.Start(ctx, id)
		case "stop":
			_, err = api.Stop(ctx, id)
		case "restart":
			_, err = api.Restart(ctx, id)
		case "remove":
			err = api.Remove(ctx, id)
		}
		return mutationMsg{kind: kind, id: id, err: err}
	}
}

func (m *Model) click(v tea.MouseClickMsg) tea.Cmd {
	if m.modal != "" || v.Button != tea.MouseLeft || m.width < 60 || m.height < 18 {
		return nil
	}
	geo := m.geometry()
	if v.Y < geo.top || v.Y >= geo.bottom {
		return nil
	}
	if geo.split && v.X == geo.leftWidth {
		return nil
	}
	if geo.split {
		if v.X < geo.leftWidth {
			m.focus = 0
		} else {
			m.focus = 1
		}
	}
	if !geo.split && m.focus == 0 || geo.split && v.X < geo.leftWidth {
		idx := m.listTop + v.Y - geo.top - 2
		visible := m.visible()
		if idx >= 0 && idx < len(visible) {
			return m.selectID(visible[idx].ID)
		}
	} else if v.Y == geo.top {
		m.detail = v.X >= geo.detailToggleX
	}
	return nil
}

func (m *Model) wheel(v tea.MouseWheelMsg) {
	if m.modal != "" || m.width < 60 || m.height < 18 {
		return
	}
	geo := m.geometry()
	if v.Y < geo.top || v.Y >= geo.bottom {
		return
	}
	if geo.split && v.X == geo.leftWidth {
		return
	}
	if geo.split && v.X < geo.leftWidth || !geo.split && m.focus == 0 {
		if v.Button == tea.MouseWheelUp {
			m.listTop = max(0, m.listTop-3)
		}
		if v.Button == tea.MouseWheelDown {
			m.listTop = min(max(0, len(m.visible())-m.listRows()), m.listTop+3)
		}
		return
	}
	if v.Button == tea.MouseWheelUp {
		m.follow = false
		m.view.ScrollUp(3)
	}
	if v.Button == tea.MouseWheelDown {
		m.view.ScrollDown(3)
	}
}
