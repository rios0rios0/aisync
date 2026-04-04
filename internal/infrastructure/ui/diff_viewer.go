package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

// BubbleteaDiffViewer displays diff results in an interactive scrollable viewport.
type BubbleteaDiffViewer struct{}

// NewBubbleteaDiffViewer creates a new BubbleteaDiffViewer.
func NewBubbleteaDiffViewer() *BubbleteaDiffViewer {
	return &BubbleteaDiffViewer{}
}

// Show renders the diff result in an interactive bubbletea viewport.
// Returns immediately if stdout is not a TTY.
func (v *BubbleteaDiffViewer) Show(result *entities.DiffResult, f entities.Formatter) error {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}

	content := BuildDiffContent(result, f)
	if content == "" {
		fmt.Println("No changes detected.")
		return nil
	}

	m := newDiffModel(content)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// BuildDiffContent formats a DiffResult into styled text for display.
func BuildDiffContent(result *entities.DiffResult, f entities.Formatter) string {
	var b strings.Builder

	writeSection := func(title string, changes []entities.FileChange) {
		if len(changes) == 0 {
			return
		}
		b.WriteString(f.Bold(title))
		b.WriteString("\n")
		for _, ch := range changes {
			symbol := f.DiffSymbol(string(ch.Direction))
			line := fmt.Sprintf("  %s %s", symbol, f.FilePath(ch.Path))
			b.WriteString(line)
			b.WriteString("\n")

			var details []string
			if ch.Source != "" {
				details = append(details, "source: "+ch.Source)
			}
			if ch.Direction == entities.ChangeModified {
				details = append(details, fmt.Sprintf("%s → %s", humanSize(ch.LocalSize), humanSize(ch.RemoteSize)))
			} else if ch.Direction == entities.ChangeAdded {
				details = append(details, humanSize(ch.RemoteSize))
			}
			if ch.Encrypted {
				details = append(details, "encrypted")
			}
			if len(details) > 0 {
				b.WriteString(fmt.Sprintf("      %s\n", f.Subtle(strings.Join(details, " | "))))
			}
		}
		b.WriteString("\n")
	}

	writeSection("External sources:", result.SharedChanges)
	writeSection("Personal (from other devices):", result.PersonalChanges)
	writeSection("Local uncommitted changes:", result.LocalUncommitted)

	return b.String()
}

func humanSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
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
