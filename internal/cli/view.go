package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/timonwong/skimi/internal/detect"
	"github.com/timonwong/skimi/internal/types"
)

// ── styles ───────────────────────────────────────────────────────────────────

var (
	viewStyleDim       = lipgloss.NewStyle().Faint(true)
	viewStyleCyanBold  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7DCFFF"))
	viewStyleDimYellow = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#E5C07B"))
	viewStyleDesc      = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#FFFFFF"))
)

// ── pager detection ──────────────────────────────────────────────────────────

type pagerKind int

const (
	pagerBat pagerKind = iota
	pagerLess
	pagerBuiltin
)

func detectPager() pagerKind {
	if _, err := exec.LookPath("bat"); err == nil {
		return pagerBat
	}
	if _, err := exec.LookPath("less"); err == nil {
		return pagerLess
	}
	return pagerBuiltin
}

// ── list item ────────────────────────────────────────────────────────────────

type skillItem struct {
	skill   types.DetectedSkill
	relPath string // filepath.Rel(sourceDir, skill.SkillPath)
}

func (si skillItem) FilterValue() string { return si.skill.Name }

// ── custom delegate (single-line, no description row) ───────────────────────

type skillDelegate struct{}

func (d skillDelegate) Height() int                               { return 1 }
func (d skillDelegate) Spacing() int                              { return 0 }
func (d skillDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d skillDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(skillItem)
	if !ok {
		return
	}
	if index == m.Index() {
		fmt.Fprintf(w, "> %s  %s",
			viewStyleCyanBold.Render(si.skill.Name),
			viewStyleDimYellow.Render(si.relPath),
		)
	} else {
		fmt.Fprintf(w, "  %s  %s",
			si.skill.Name,
			viewStyleDim.Render(si.relPath),
		)
	}
}

// ── bubbletea model ──────────────────────────────────────────────────────────

type viewState int

const (
	stateList  viewState = iota
	statePager           // builtin viewport pager
)

type pagerDoneMsg struct{ err error }

const descAreaHeight = 2 // 1 separator line + 1 description line

type viewModel struct {
	state       viewState
	list        list.Model
	viewport    viewport.Model
	pagerKind   pagerKind
	focusedDesc string
	width       int
	height      int
	quitting    bool
}

func newViewModel(skills []types.DetectedSkill, sourceDir string, pk pagerKind) viewModel {
	items := make([]list.Item, len(skills))
	for i, s := range skills {
		rel, err := filepath.Rel(sourceDir, s.SkillPath)
		if err != nil {
			rel = s.SkillPath
		}
		items[i] = skillItem{skill: s, relPath: rel}
	}

	l := list.New(items, skillDelegate{}, 80, 20)
	l.Title = "Select a skill to view"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	// Disable built-in quit keys — we handle q and ctrl+c ourselves.
	l.KeyMap.Quit.SetEnabled(false)
	l.KeyMap.ForceQuit.SetEnabled(false)

	// Inject q into the help bar.
	qBinding := key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))
	l.AdditionalShortHelpKeys = func() []key.Binding { return []key.Binding{qBinding} }
	l.AdditionalFullHelpKeys = func() []key.Binding { return []key.Binding{qBinding} }

	var focusedDesc string
	if sel := l.SelectedItem(); sel != nil {
		focusedDesc = sel.(skillItem).skill.Description
	}

	return viewModel{
		list:        l,
		pagerKind:   pk,
		focusedDesc: focusedDesc,
	}
}

func (m viewModel) Init() tea.Cmd { return nil }

func (m viewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listH := m.height - descAreaHeight
		if listH < 1 {
			listH = 1
		}
		m.list.SetSize(m.width, listH)
		if m.state == statePager {
			m.viewport.Width = m.width
			m.viewport.Height = m.height
		}
		return m, nil

	case tea.KeyMsg:
		if m.state == stateList {
			switch msg.String() {
			case "q", "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "enter":
				sel := m.list.SelectedItem()
				if sel == nil {
					return m, nil
				}
				var cmd tea.Cmd
				m, cmd = m.openPager(sel.(skillItem))
				return m, cmd
			}
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			if sel := m.list.SelectedItem(); sel != nil {
				m.focusedDesc = sel.(skillItem).skill.Description
			}
			return m, cmd
		}

		// statePager (builtin viewport)
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.state = stateList
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case pagerDoneMsg:
		// External pager (bat/less) finished — return to list.
		m.state = stateList
		return m, nil
	}

	// Default forwarding.
	if m.state == stateList {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	if m.state == statePager {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m viewModel) View() string {
	if m.quitting {
		return ""
	}
	if m.state == statePager {
		return m.viewport.View()
	}
	sep := viewStyleDim.Render(strings.Repeat("─", m.width))
	desc := viewStyleDesc.Render(m.focusedDesc)
	return m.list.View() + "\n" + sep + "\n" + desc
}

// openPager sets up the pager for the selected skill and returns the updated
// model and the command to run (or nil for the builtin viewport).
func (m viewModel) openPager(si skillItem) (viewModel, tea.Cmd) {
	path := filepath.Join(si.skill.SkillPath, "SKILL.md")
	m.state = statePager
	switch m.pagerKind {
	case pagerBat:
		c := exec.Command("bat", "--style=plain", "--paging=always", path)
		return m, tea.ExecProcess(c, func(err error) tea.Msg { return pagerDoneMsg{err} })
	case pagerLess:
		c := exec.Command("less", path)
		return m, tea.ExecProcess(c, func(err error) tea.Msg { return pagerDoneMsg{err} })
	case pagerBuiltin:
		content, _ := os.ReadFile(path)
		m.viewport = viewport.New(m.width, m.height)
		m.viewport.SetContent(string(content))
		return m, nil
	}
	return m, nil
}

// ── single-skill shortcut (no TUI) ───────────────────────────────────────────

func openSingleSkill(si skillItem, pk pagerKind) error {
	path := filepath.Join(si.skill.SkillPath, "SKILL.md")
	switch pk {
	case pagerBat:
		c := exec.Command("bat", "--style=plain", "--paging=always", path)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		return c.Run()
	case pagerLess:
		c := exec.Command("less", path)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		return c.Run()
	case pagerBuiltin:
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Print(string(content))
		return nil
	}
	return nil
}

// ── command wiring ────────────────────────────────────────────────────────────

func newViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "view <source>",
		Short: "Preview skills available in a source without installing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runView(args[0], globalStoreDir)
		},
	}
}

func runView(source, storeDir string) error {
	sourceDir, _, err := resolveSource(source, storeDir)
	if err != nil {
		return err
	}

	skills, err := detect.Scan(sourceDir)
	if err != nil {
		return fmt.Errorf("detect skills: %w", err)
	}
	if len(skills) == 0 {
		fmt.Println("No skills found.")
		return nil
	}

	pk := detectPager()

	// Single skill: skip the list TUI and open directly.
	if len(skills) == 1 {
		rel, _ := filepath.Rel(sourceDir, skills[0].SkillPath)
		si := skillItem{skill: skills[0], relPath: rel}
		return openSingleSkill(si, pk)
	}

	m := newViewModel(skills, sourceDir, pk)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI: %w", err)
	}
	return nil
}
