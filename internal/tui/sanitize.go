package tui

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// safeText turns untrusted bytes into visible, inert terminal text. It is
// presentation-only: the API client's byte buffer and offsets are untouched.
func safeText(raw []byte) string {
	var b strings.Builder
	b.Grow(min(len(raw)*2, 256*1024))
	for len(raw) > 0 {
		r, n := utf8.DecodeRune(raw)
		if r == utf8.RuneError && n == 1 {
			fmt.Fprintf(&b, "\\x%02X", raw[0])
		} else if r == '\n' {
			b.WriteByte('\n')
		} else if r == '\t' {
			b.WriteString("    ")
		} else if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			if r <= 0xff {
				fmt.Fprintf(&b, "\\x%02X", r)
			} else {
				fmt.Fprintf(&b, "\\u%04X", r)
			}
		} else {
			b.WriteRune(r)
		}
		raw = raw[n:]
	}
	return b.String()
}

func safeLine(s string) string { return strings.ReplaceAll(safeText([]byte(s)), "\n", "\\n") }

// cellClip clips without splitting a wide rune; the caller pads separately.
func cellClip(s string, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := lipgloss.Width(string(r))
		if used+w > width {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String()
}

func padded(s string, width int) string {
	s = cellClip(s, width)
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}
