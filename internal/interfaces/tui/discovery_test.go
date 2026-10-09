package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"cserver/internal/app"
	"cserver/internal/core"
	"cserver/internal/service"
)

type tuiDiscoveryReader struct {
	servers []core.DiscoveredServer
	err     error
	calls   int
}

func (f *tuiDiscoveryReader) Discover(
	_ context.Context,
	_ string,
) ([]core.DiscoveredServer, error) {
	f.calls++
	return f.servers, f.err
}

func discoveryModel(
	t *testing.T,
	reader *tuiDiscoveryReader,
) Model {
	t.Helper()

	discovery, err := service.NewServerDiscoveryService(
		t.TempDir(),
		reader,
	)
	if err != nil {
		t.Fatal(err)
	}

	return NewModel(app.App{
		Info:      service.GetApplicationInfo(),
		Discovery: discovery,
	})
}

func openServers(m Model) (Model, tea.Cmd) {
	m, _ = pressKey(m, tea.Key{Code: tea.KeyDown})
	m, _ = pressKey(m, tea.Key{Code: tea.KeyDown})

	return pressKey(m, tea.Key{Code: tea.KeyEnter})
}

func TestServersScreenLoadsAsynchronously(t *testing.T) {
	reader := &tuiDiscoveryReader{
		servers: []core.DiscoveredServer{
			{
				ID:     "aim",
				Name:   "AIM Server",
				Status: core.DiscoveryComplete,
			},
		},
	}

	m, cmd := openServers(discoveryModel(t, reader))

	if m.screen != screenServers ||
		!m.serverLoading ||
		cmd == nil {
		t.Fatal("expected asynchronous discovery command")
	}

	if reader.calls != 0 {
		t.Fatal("discovery ran inside the event handler")
	}

	updated, _ := m.Update(cmd())
	m = updated.(Model)

	if m.serverLoading ||
		reader.calls != 1 ||
		!strings.Contains(m.render(), "AIM Server") {
		t.Fatalf("unexpected loaded screen: %s", m.render())
	}
}

func TestServersRefresh(t *testing.T) {
	reader := &tuiDiscoveryReader{}

	m, first := openServers(discoveryModel(t, reader))

	updated, _ := m.Update(first())
	m = updated.(Model)

	m, refresh := pressKey(m, tea.Key{Code: 'r'})

	if refresh == nil || !m.serverLoading {
		t.Fatal("expected refresh command")
	}

	updated, _ = m.Update(refresh())
	m = updated.(Model)

	if reader.calls != 2 ||
		m.serverLoading ||
		!strings.Contains(
			m.render(),
			"No CS2 instances found",
		) {
		t.Fatalf("unexpected refreshed screen: %s", m.render())
	}
}

func TestServersIgnoreStaleResults(t *testing.T) {
	reader := &tuiDiscoveryReader{}

	m, first := openServers(discoveryModel(t, reader))
	m, second := pressKey(m, tea.Key{Code: 'r'})

	// The older command returns after a newer request was made.
	updated, _ := m.Update(first())
	m = updated.(Model)

	if !m.serverLoading {
		t.Fatal("stale result incorrectly completed refresh")
	}

	updated, _ = m.Update(second())
	m = updated.(Model)

	if m.serverLoading {
		t.Fatal("current result did not complete refresh")
	}

	m, _ = pressKey(m, tea.Key{Code: tea.KeyEsc})

	if m.screen != screenHome {
		t.Fatal("expected return to home")
	}
}

func TestServersScreenError(t *testing.T) {
	reader := &tuiDiscoveryReader{
		err: errors.New("fixture discovery error"),
	}

	m, cmd := openServers(discoveryModel(t, reader))

	updated, _ := m.Update(cmd())
	m = updated.(Model)

	if !strings.Contains(
		m.render(),
		"fixture discovery error",
	) {
		t.Fatalf("expected error screen: %s", m.render())
	}
}

func TestServersScreenWithoutService(t *testing.T) {
	m := NewModel(app.App{
		Info: service.GetApplicationInfo(),
	})

	m, cmd := openServers(m)

	updated, _ := m.Update(cmd())
	m = updated.(Model)

	if !strings.Contains(
		m.render(),
		"discovery service is unavailable",
	) {
		t.Fatalf("expected unavailable message: %s", m.render())
	}
}
