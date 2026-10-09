package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"cserver/internal/app"
	"cserver/internal/config"
	"cserver/internal/service"
)

func testModel() Model {
	return NewModel(app.App{
		Config: config.Config{
			ServersRoot: "/test/servers",
		},
		Info: service.GetApplicationInfo(),
	})
}

func pressKey(m Model, key tea.Key) (Model, tea.Cmd) {
	updated, cmd := m.Update(tea.KeyPressMsg(key))
	return updated.(Model), cmd
}

func TestInitialScreen(t *testing.T) {
	m := testModel()

	if m.screen != screenHome {
		t.Fatal("expected home screen")
	}

	if m.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.cursor)
	}
}

func TestMenuNavigation(t *testing.T) {
	m := testModel()

	m, _ = pressKey(m, tea.Key{Code: tea.KeyDown})

	if m.cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", m.cursor)
	}

	m, _ = pressKey(m, tea.Key{Code: tea.KeyUp})

	if m.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.cursor)
	}
}

func TestMenuBounds(t *testing.T) {
	m := testModel()

	m, _ = pressKey(m, tea.Key{Code: tea.KeyUp})

	if m.cursor != 0 {
		t.Fatal("cursor moved above first item")
	}

	for range 10 {
		m, _ = pressKey(m, tea.Key{Code: tea.KeyDown})
	}

	if m.cursor != len(menuItems)-1 {
		t.Fatal("cursor moved beyond last item")
	}
}

func TestOpenInformationScreen(t *testing.T) {
	m := testModel()

	m, _ = pressKey(m, tea.Key{Code: tea.KeyEnter})

	if m.screen != screenInformation {
		t.Fatal("expected information screen")
	}

	if !strings.Contains(m.render(), service.Version) {
		t.Fatal("information screen does not show version")
	}
}

func TestOpenConfigurationScreen(t *testing.T) {
	m := testModel()

	m, _ = pressKey(m, tea.Key{Code: tea.KeyDown})
	m, _ = pressKey(m, tea.Key{Code: tea.KeyEnter})

	if m.screen != screenConfiguration {
		t.Fatal("expected configuration screen")
	}

	if !strings.Contains(m.render(), "/test/servers") {
		t.Fatal("configuration screen does not show root")
	}
}

func TestBackNavigation(t *testing.T) {
	m := testModel()

	m, _ = pressKey(m, tea.Key{Code: tea.KeyEnter})
	m, _ = pressKey(m, tea.Key{Code: tea.KeyEsc})

	if m.screen != screenHome {
		t.Fatal("expected return to home screen")
	}
}

func TestQuitCommand(t *testing.T) {
	m := testModel()

	_, cmd := pressKey(m, tea.Key{Code: 'q'})

	if cmd == nil {
		t.Fatal("expected quit command")
	}

	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected QuitMsg")
	}
}

func TestWindowResize(t *testing.T) {
	m := testModel()

	updated, _ := m.Update(tea.WindowSizeMsg{
		Width:  120,
		Height: 40,
	})

	m = updated.(Model)

	if m.width != 120 || m.height != 40 {
		t.Fatal("window dimensions were not updated")
	}
}
