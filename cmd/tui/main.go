package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	celgo "github.com/google/cel-go/cel"
	"github.com/lukasschwab/feedcel/pkg/cel"
	"github.com/lukasschwab/feedcel/pkg/feed"
	"github.com/mmcdole/gofeed"
)

var (
	appStyle             lipgloss.Style
	leftPaneStyle        lipgloss.Style
	rightPaneStyle       lipgloss.Style
	titleStyle           lipgloss.Style
	errorStyle           lipgloss.Style
	separatorStyle       lipgloss.Style
	activeSeparatorStyle lipgloss.Style
	activeInputStyle     lipgloss.Style
)

func init() {
	appStyle = lipgloss.NewStyle().Margin(1, 2)
	leftPaneStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).MarginRight(1)
	rightPaneStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(0, 1)
	titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFF7DB")).Background(lipgloss.Color("#888B7E")).Padding(0, 1)
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Padding(1)
	separatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Padding(0, 1)
	activeSeparatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Padding(0, 1)
	activeInputStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
}

type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Enter  key.Binding
	Tab    key.Binding
	Esc    key.Binding
	Help   key.Binding
	Schema key.Binding
	Quit   key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "move up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "move down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select/submit"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "toggle focus"),
	),
	Esc: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "blur input"),
	),
	Schema: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "toggle schema"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Tab, k.Esc, k.Schema, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter},
		{k.Tab, k.Esc, k.Schema, k.Quit},
	}
}

// Simple item wrapper to satisfy list.Item
type expressionItem struct {
	expr   string
	status string
}

func (e expressionItem) Title() string       { return e.expr }
func (e expressionItem) Description() string { return e.status }
func (e expressionItem) FilterValue() string { return e.expr }

func main() {
	url := flag.String("url", "", "URL of the feed to inspect")
	flag.Parse()

	if *url == "" {
		fmt.Println("Please provide a feed URL with -url")
		os.Exit(1)
	}

	// Fetch Feed
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fp := gofeed.NewParser()
	parsed, err := fp.ParseURLWithContext(*url, ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Setup CEL Env
	env, err := cel.NewEnv()
	if err != nil {
		log.Fatal(err)
	}

	m := initialModel(parsed, env)
	finalModel, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}

	// Print valid expressions on exit
	if finalM, ok := finalModel.(model); ok {
		for _, item := range finalM.list.Items() {
			if exprItem, ok := item.(expressionItem); ok {
				// Filter out empty or error? Or print all? User said "didn't result in errors".
				if !strings.HasPrefix(exprItem.status, "Error") {
					fmt.Printf("%s\n\t[%s]\n", exprItem.expr, exprItem.status)
				}
			}
		}
	}
}

type model struct {
	feed *gofeed.Feed
	env  *celgo.Env

	list      list.Model
	textInput textinput.Model
	viewport  viewport.Model
	help      help.Model

	width  int
	height int

	lastExpr   string
	showSchema bool
}

func initialModel(f *gofeed.Feed, env *celgo.Env) model {
	m := model{
		feed:      f,
		env:       env,
		textInput: textinput.New(),
		viewport:  viewport.New(0, 0),
		help:      help.New(),
		lastExpr:  "",
	}

	// List setup
	status := m.evaluateExpression("true")
	items := []list.Item{
		expressionItem{expr: "true", status: status},
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "CEL Expressions"
	l.SetShowHelp(false)
	m.list = l

	// Input setup
	m.textInput.Placeholder = "Enter CEL expression..."
	m.textInput.Focus()
	m.textInput.CharLimit = 156
	m.textInput.Width = 30
	m.textInput.PromptStyle = activeInputStyle

	return m
}

func (m model) evaluateExpression(expr string) string {
	prg, err := cel.Compile(m.env, expr)
	if err != nil {
		return "Error: Compile failed"
	}

	count := 0
	now := time.Now()
	for _, item := range m.feed.Items {
		celItem := feed.Transform(item)
		match, err := cel.Evaluate(prg, celItem, now)
		if err == nil && match {
			count++
		}
	}
	return fmt.Sprintf("%d matches", count)
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Esc):
			if m.textInput.Focused() {
				m.textInput.Blur()
				m.textInput.PromptStyle = lipgloss.NewStyle()
			}
			return m, nil
		case key.Matches(msg, keys.Tab):
			if m.textInput.Focused() {
				m.textInput.Blur()
				m.textInput.PromptStyle = lipgloss.NewStyle()
			} else {
				m.textInput.Focus()
				m.textInput.PromptStyle = activeInputStyle
			}
		case key.Matches(msg, keys.Enter):
			if m.textInput.Focused() {
				v := m.textInput.Value()
				if v != "" {
					status := m.evaluateExpression(v)
					m.list.InsertItem(len(m.list.Items()), expressionItem{expr: v, status: status})
					m.list.Select(len(m.list.Items()) - 1)
					m.textInput.SetValue("")
					// Clear filter if any
					m.list.ResetFilter()
				}
			} else {
				m.textInput.Focus()
				m.textInput.PromptStyle = activeInputStyle
			}
		case key.Matches(msg, keys.Schema):
			if !m.textInput.Focused() {
				m.showSchema = !m.showSchema
				m.lastExpr = "" // force rerender
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		footerHeight := 1
		availableHeight := m.height - footerHeight
		leftWidth := int(float64(m.width) * 0.3)
		// Reduced rightWidth by extra 1 to avoid cut-off
		rightWidth := m.width - leftWidth - 2

		m.textInput.Width = leftWidth - 4
		inputHeight := lipgloss.Height(m.textInput.View())
		separatorHeight := 1
		listHeight := availableHeight - inputHeight - separatorHeight - 2
		m.list.SetSize(leftWidth-2, listHeight)
		m.viewport.Width = rightWidth - 2
		m.viewport.Height = availableHeight - 2
		m.help.Width = m.width
	}

	// Update components
	var cmd tea.Cmd

	if m.textInput.Focused() {
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	// Update Content
	selectedItem := m.list.SelectedItem()
	if selectedItem != nil {
		expr := selectedItem.(expressionItem).expr
		if m.showSchema {
			if m.lastExpr != "SCHEMA" {
				m.lastExpr = "SCHEMA"
				m.viewport.SetContent(m.renderSchema())
			}
		} else if expr != m.lastExpr {
			m.lastExpr = expr
			content := m.renderContent(expr)
			m.viewport.SetContent(content)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) renderSchema() string {
	schema := `CEL Environment Schema (item variable):

- URL           (string)
- Title         (string)
- Author        (string)
- Tags          (string)
- Content       (string)
- ContentLength (int)
- Published     (timestamp)
- Updated       (timestamp)

Global variables:
- now           (timestamp)`
	return titleStyle.Render("Schema Inspector") + "\n\n" + schema
}

func (m model) renderContent(expr string) string {
	prg, err := cel.Compile(m.env, expr)
	if err != nil {
		return errorStyle.Render(fmt.Sprintf("Compilation Error:\n%v", err))
	}

	var sb strings.Builder
	now := time.Now()

	count := 0
	for _, item := range m.feed.Items {
		celItem := feed.Transform(item)
		match, err := cel.Evaluate(prg, celItem, now)
		if err != nil {
			sb.WriteString(errorStyle.Render(fmt.Sprintf("Evaluation Error on item '%s': %v\n", item.Title, err)))
			continue
		}

		if match {
			count++
			sb.WriteString(lipgloss.NewStyle().Bold(true).Render(item.Title) + "\n")
			if item.Author != nil {
				sb.WriteString(fmt.Sprintf("By %s\n", item.Author.Name))
			}
			sb.WriteString(fmt.Sprintf("%s\n", item.Link))
			if len(item.Categories) > 0 {
				sb.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(item.Categories, ", ")))
			}
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("---") + "\n")
		}
	}

	// Updated header with URL
	header := titleStyle.Render(fmt.Sprintf("Matches: %d  |  URL: %s", count, m.feed.Link))
	return header + "\n\n" + sb.String()
}

func (m model) View() string {
	footerHeight := 1
	availableHeight := m.height - footerHeight

	sepStyle := separatorStyle
	if m.textInput.Focused() {
		sepStyle = activeSeparatorStyle
	}
	separator := sepStyle.Render(strings.Repeat("─", m.list.Width()))

	left := leftPaneStyle.Height(availableHeight - 2).Render(lipgloss.JoinVertical(lipgloss.Left,
		m.list.View(),
		separator,
		m.textInput.View(),
	))

	right := rightPaneStyle.Height(availableHeight - 2).Render(m.viewport.View())

	mainView := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	helpView := m.help.View(keys)

	return lipgloss.JoinVertical(lipgloss.Left, mainView, helpView)
}
