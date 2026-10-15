package ui

import (
	"errors"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// errPrintInstead tells the caller to print the content itself: there is no
// terminal to page in, or the content already fits on the screen.
var errPrintInstead = errors.New("print the content instead of paging it")

// BubbleteaDiffViewer pages diff output in an interactive scrollable viewport.
type BubbleteaDiffViewer struct{}

// NewBubbleteaDiffViewer creates a new BubbleteaDiffViewer.
func NewBubbleteaDiffViewer() *BubbleteaDiffViewer {
	return &BubbleteaDiffViewer{}
}

// ShowText pages content in a scrollable viewport when it is taller than the
// terminal. Otherwise it returns an error without showing anything, so the
// caller prints the content and it stays visible after the command exits.
func (v *BubbleteaDiffViewer) ShowText(content string) error {
	_, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || strings.Count(content, "\n") < height-1 {
		return errPrintInstead
	}
	_, err = tea.NewProgram(newDiffModel(content), tea.WithAltScreen()).Run()
	return err
}

// diffModel is the bubbletea model for the interactive diff viewport.
type diffModel struct {
	viewport viewport.Model
	content  string
	ready    bool
}

func newDiffModel(content string) diffModel {
	return diffModel{content: content}
}

//nolint:gochecknoglobals // lipgloss styles are compile-time constants
var helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

func (m diffModel) Init() tea.Cmd {
	return nil
}

func (m diffModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-1)
			m.viewport.SetContent(m.content)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 1
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m diffModel) View() string {
	if !m.ready {
		return "Loading..."
	}
	footer := helpStyle.Render("↑/↓ scroll • q quit")
	return m.viewport.View() + "\n" + footer
}
