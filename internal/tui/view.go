package tui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type bounds struct {
	split                                                         bool
	leftWidth, rightStart, rightWidth, top, bottom, detailToggleX int
}

func (m *Model) geometry() bounds {
	split := m.width >= 100
	left := m.width
	rightStart := 0
	rightWidth := m.width
	if split {
		left = m.width * 40 / 100
		rightStart = left + 1
		rightWidth = m.width - rightStart
	}
	return bounds{split: split, leftWidth: left, rightStart: rightStart, rightWidth: rightWidth, top: 2, bottom: max(2, m.height-2), detailToggleX: rightStart + 11}
}

func (m *Model) listRows() int { g := m.geometry(); return max(1, g.bottom-g.top-3) }

func (m *Model) resizeViewport() {
	g := m.geometry()
	m.view.SetWidth(max(1, g.rightWidth-2))
	m.view.SetHeight(max(1, g.bottom-g.top-4))
	if m.follow {
		m.view.GotoBottom()
	}
	for i := range m.form {
		m.form[i].SetWidth(max(12, m.width-18))
	}
}

func (m *Model) View() tea.View {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		return tea.NewView("")
	}
	if w < 60 || h < 18 {
		lines := make([]string, h)
		lines[0] = padded("CS2M — resize to at least 60×18; Ctrl+C or q quits", w)
		for i := 1; i < h; i++ {
			lines[i] = strings.Repeat(" ", w)
		}
		v := tea.NewView(strings.Join(lines, "\n"))
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}
	g := m.geometry()
	lines := make([]string, 0, h)
	running := 0
	for _, i := range m.instances {
		if i.State == "running" {
			running++
		}
	}
	status := string(m.status)
	header := fmt.Sprintf("CS2M / SERVER MANAGER  [%s]  INSTANCES %d  RUNNING %d", status, len(m.instances), running)
	lines = append(lines, m.styleHeader(padded(safeLine(header), w)))
	line2 := "API  " + safeLine(m.endpoint)
	if m.filtering {
		line2 += "  |  " + renderInput(m.filter, max(1, w-lipgloss.Width(line2)-5))
	} else if m.filter.Value() != "" {
		line2 += "  |  FILTER /" + safeLine(m.filter.Value())
	}
	lines = append(lines, padded(line2, w))
	left := m.listPanel(g.leftWidth, g.bottom-g.top)
	right := m.detailPanel(g.rightWidth, g.bottom-g.top)
	for i := 0; i < g.bottom-g.top; i++ {
		if g.split {
			line := left[i] + " " + right[i]
			if i == 0 {
				line = m.styleHeader(line)
			}
			lines = append(lines, line)
		} else if m.focus == 0 {
			line := left[i]
			if i == 0 {
				line = m.styleHeader(line)
			}
			lines = append(lines, line)
		} else {
			line := right[i]
			if i == 0 {
				line = m.styleHeader(line)
			}
			lines = append(lines, line)
		}
	}
	footer := "Tab panel  n new  s start  x stop  r restart  d remove  v logs/details  ? help  q quit"
	if m.modal == "form" {
		footer = "Tab fields  Ctrl+A add argument  Ctrl+D remove argument  Enter next/submit  Esc cancel"
	}
	if m.modal != "" && m.modal != "form" {
		footer = "y confirm  n/Esc cancel"
	}
	lines = append(lines, m.styleHeader(padded("KEYS  "+footer, w)))
	lines = append(lines, padded("STATUS  "+safeLine(m.message), w))
	if m.help {
		m.overlay(lines, []string{"HELP  Esc/? close", "List: j/k or arrows, / filter, Enter detail", "Logs: j/k, PgUp/PgDown, g/G, f follow", "Tab changes focus; v switches Logs/Details", "n register; s start; x stop; r restart; d remove", "R reconnect; L reload retained logs; q quit"})
	}
	if m.modal == "form" {
		content := []string{fmt.Sprintf("REGISTER INSTANCE — field %d/%d", m.formIndex+1, len(m.form))}
		maxFields := max(1, min(len(m.form), h-8))
		first := max(0, min(m.formIndex-maxFields/2, len(m.form)-maxFields))
		for i := first; i < min(len(m.form), first+maxFields); i++ {
			f := m.form[i]
			prefix := "  "
			if i == m.formIndex {
				prefix = "> "
			}
			content = append(content, prefix+renderInput(f, max(1, w-4)))
		}
		content = append(content, "Ctrl+A add argument; Ctrl+D remove current argument")
		m.overlay(lines, content)
	} else if m.modal != "" {
		m.overlay(lines, []string{strings.ToUpper(m.modal) + " " + safeLine(m.modalID) + "?", "Press y to confirm or n/Esc to cancel"})
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func renderInput(f textinput.Model, width int) string {
	runes := []rune(f.Value())
	pos := max(0, min(f.Position(), len(runes)))
	// Keep the insertion point visible for long paths without exposing control
	// bytes from either the prompt or the user's input to the terminal.
	prefix := safeLine(f.Prompt)
	available := max(1, width-lipgloss.Width(prefix)-2)
	start := 0
	for start < pos && lipgloss.Width(safeLine(string(runes[start:pos]))) > available/2 {
		start++
	}
	text := safeLine(string(runes[start:pos])) + "|" + safeLine(string(runes[pos:]))
	return prefix + cellClip(text, available+1)
}

func (m *Model) styleHeader(s string) string {
	if m.mono || os.Getenv("NO_COLOR") != "" {
		return lipgloss.NewStyle().Bold(true).Render(s)
	}
	choose := lipgloss.LightDark(m.dark)
	return lipgloss.NewStyle().Bold(true).Foreground(choose(lipgloss.Color("#17427a"), lipgloss.Color("#a9d6ff"))).Render(s)
}

func (m *Model) listPanel(width, height int) []string {
	out := make([]string, height)
	if height < 2 {
		return out
	}
	out[0] = panelTop(width, "INSTANCES", m.focus == 0)
	out[height-1] = panelBottom(width, m.focus == 0)
	inner := max(0, width-2)
	stateWidth := 10
	portWidth := 5
	idWidth := max(1, inner-2-stateWidth-portWidth-2)
	header := "  " + padded("ID", idWidth) + " " + padded("STATE", stateWidth) + " " + padded("PORT", portWidth)
	out[1] = panelBody(width, header, m.focus == 0)
	visible := m.visible()
	for row := 2; row < height-1; row++ {
		idx := m.listTop + row - 2
		if idx >= len(visible) {
			out[row] = panelBody(width, "", m.focus == 0)
			continue
		}
		i := visible[idx]
		mark := " "
		if i.ID == m.selected {
			mark = ">"
		}
		id := truncated(safeLine(i.ID), idWidth)
		state := truncated(strings.ToUpper(safeLine(i.State)), stateWidth)
		line := mark + " " + padded(id, idWidth) + " " + padded(state, stateWidth) + " " + padded(fmt.Sprint(i.Port), portWidth)
		out[row] = panelBody(width, line, m.focus == 0)
	}
	return out
}

func (m *Model) detailPanel(width, height int) []string {
	out := make([]string, height)
	if height < 2 {
		return out
	}
	title := "[LOGS]   DETAILS  (v)"
	if m.detail {
		title = " LOGS   [DETAILS] (v)"
	}
	out[0] = panelTop(width, title, m.focus == 1)
	out[height-1] = panelBottom(width, m.focus == 1)
	inner := max(0, width-2)
	selected, ok := m.selectedInstance()
	if !ok {
		out[1] = panelBody(width, "Select an instance", m.focus == 1)
		for i := 2; i < height-1; i++ {
			out[i] = panelBody(width, "", m.focus == 1)
		}
		return out
	}
	port := fmt.Sprintf("PORT %d", selected.Port)
	idWidth := max(1, inner-lipgloss.Width(port)-4)
	out[1] = panelBody(width, "ID "+padded(truncated(safeLine(selected.ID), idWidth), idWidth)+" "+port, m.focus == 1)
	if m.detail {
		out[2] = panelBody(width, "STATE "+strings.ToUpper(safeLine(selected.State)), m.focus == 1)
		if height > 4 {
			out[3] = panelBody(width, "ERROR "+safeLine(selected.Error), m.focus == 1)
		}
		if height > 5 {
			out[4] = panelBody(width, "LAST TERMINATION "+safeLine(selected.LastTermination), m.focus == 1)
		}
		for i := 5; i < height-1; i++ {
			out[i] = panelBody(width, "", m.focus == 1)
		}
		return out
	}
	mode := "PAUSED"
	if m.follow {
		mode = "FOLLOW"
	}
	out[2] = panelBody(width, "STATE "+strings.ToUpper(safeLine(selected.State))+"  |  LOGS "+mode+" (f)", m.focus == 1)
	viewLines := strings.Split(m.view.View(), "\n")
	for i := 3; i < height-1; i++ {
		if i-3 < len(viewLines) {
			out[i] = panelBody(width, viewLines[i-3], m.focus == 1)
		} else {
			out[i] = panelBody(width, "", m.focus == 1)
		}
	}
	return out
}

func panelTop(width int, title string, focus bool) string {
	left, right, rule := "┌", "┐", "─"
	label := " " + title + " "
	if focus {
		left, right, rule = "┏", "┓", "━"
		label = " " + title + "  FOCUSED "
	}
	label = cellClip(label, max(0, width-2))
	return left + label + strings.Repeat(rule, max(0, width-2-lipgloss.Width(label))) + right
}

func panelBottom(width int, focus bool) string {
	left, right, rule := "└", "┘", "─"
	if focus {
		left, right, rule = "┗", "┛", "━"
	}
	return left + strings.Repeat(rule, max(0, width-2)) + right
}

func panelBody(width int, s string, focus bool) string {
	bar := "│"
	if focus {
		bar = "┃"
	}
	return bar + padded(s, max(0, width-2)) + bar
}

func truncated(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	return cellClip(s, width-1) + "…"
}

func (m *Model) overlay(lines []string, content []string) {
	if len(lines) < 3 {
		return
	}
	start := max(2, (len(lines)-len(content))/2)
	for i, s := range content {
		if start+i >= len(lines)-2 {
			break
		}
		lines[start+i] = padded("  "+safeLine(s), m.width)
	}
}
