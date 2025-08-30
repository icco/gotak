package main

import (
	"flag"
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	localFlag = flag.Bool("local", false, "Use local server instead of https://gotak.app")
	
	// Styles
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		MarginLeft(2)
		
	menuItemStyle = lipgloss.NewStyle().
		MarginLeft(2)
		
	selectedMenuItemStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("170")).
		Bold(true).
		MarginLeft(2)
		
	boardStyle = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1).
		MarginLeft(2)
		
	cellStyle = lipgloss.NewStyle().
		Width(3).
		Height(1).
		Align(lipgloss.Center)
		
	errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true).
		MarginLeft(2)
)

func main() {
	flag.Parse()
	
	var serverURL string
	if *localFlag {
		serverURL = "http://localhost:8080"
	} else {
		serverURL = "https://gotak.app"
	}

	p := tea.NewProgram(
		initialModel(serverURL),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

type screen int

const (
	screenMenu screen = iota
	screenGame
	screenSettings
)

type model struct {
	serverURL string
	screen    screen
	
	// Menu state
	menuCursor int
	
	// Game state
	gameSlug   string
	boardSize  int
	difficulty string
	game       *GameState
	
	// UI state
	width  int
	height int
	error  string
}

type GameState struct {
	Board   [][]string
	Status  string
	Turn    int
	Player  int
	Winner  int
}

func initialModel(serverURL string) model {
	return model{
		serverURL:  serverURL,
		screen:     screenMenu,
		menuCursor: 0,
		boardSize:  5,
		difficulty: "intermediate",
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
		
	case gameStarted:
		m.game = msg.game
		return m, nil
		
	case tea.KeyMsg:
		switch m.screen {
		case screenMenu:
			return m.updateMenu(msg)
		case screenGame:
			return m.updateGame(msg)
		case screenSettings:
			return m.updateSettings(msg)
		}
	}
	
	return m, nil
}

func (m model) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
		
	case "up", "k":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
		
	case "down", "j":
		if m.menuCursor < 2 { // 3 menu items
			m.menuCursor++
		}
		
	case "enter", " ":
		switch m.menuCursor {
		case 0: // New Game
			m.screen = screenGame
			return m, m.startNewGame()
		case 1: // Settings
			m.screen = screenSettings
		case 2: // Quit
			return m, tea.Quit
		}
	}
	
	return m, nil
}

func (m model) updateGame(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.screen = screenMenu
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	
	return m, nil
}

func (m model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.screen = screenMenu
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	
	return m, nil
}

func (m model) View() string {
	switch m.screen {
	case screenMenu:
		return m.viewMenu()
	case screenGame:
		return m.viewGame()
	case screenSettings:
		return m.viewSettings()
	default:
		return "Unknown screen"
	}
}

func (m model) viewMenu() string {
	title := titleStyle.Render("🎯 GoTak - A Tak Game Implementation")
	
	choices := []string{
		"🎮 New Game",
		"⚙️  Settings", 
		"🚪 Quit",
	}
	
	menu := ""
	for i, choice := range choices {
		if m.menuCursor == i {
			menu += selectedMenuItemStyle.Render(fmt.Sprintf("> %s", choice)) + "\n"
		} else {
			menu += menuItemStyle.Render(fmt.Sprintf("  %s", choice)) + "\n"
		}
	}
	
	info := menuItemStyle.Render(fmt.Sprintf("Server: %s", m.serverURL))
	help := menuItemStyle.Render("Press ↑/↓ to navigate, Enter to select, q to quit")
	
	content := lipgloss.JoinVertical(lipgloss.Left, title, "", menu, info, "", help)
	
	if m.error != "" {
		errorMsg := errorStyle.Render(fmt.Sprintf("❌ Error: %s", m.error))
		content = lipgloss.JoinVertical(lipgloss.Left, content, "", errorMsg)
	}
	
	return content
}

func (m model) viewGame() string {
	title := titleStyle.Render("🎯 GoTak Game")
	
	if m.game == nil {
		loading := menuItemStyle.Render("Starting game...")
		help := menuItemStyle.Render("Press q to go back to menu")
		return lipgloss.JoinVertical(lipgloss.Left, title, "", loading, "", help)
	}
	
	// Render board
	board := m.renderBoard()
	gameInfo := menuItemStyle.Render(fmt.Sprintf("Turn: %d | Player: %d | Status: %s", 
		m.game.Turn, m.game.Player, m.game.Status))
	help := menuItemStyle.Render("Press q to go back to menu")
	
	return lipgloss.JoinVertical(lipgloss.Left, title, "", gameInfo, "", board, "", help)
}

func (m model) viewSettings() string {
	s := "\n⚙️ Settings\n\n"
	s += fmt.Sprintf("Board Size: %dx%d\n", m.boardSize, m.boardSize)
	s += fmt.Sprintf("AI Difficulty: %s\n", m.difficulty)
	s += fmt.Sprintf("Server: %s\n", m.serverURL)
	s += "\nPress q to go back to menu"
	
	return s
}

func (m model) renderBoard() string {
	if m.game == nil || m.game.Board == nil {
		return "No game board"
	}
	
	s := ""
	size := len(m.game.Board)
	
	// Column headers
	s += "   "
	for i := 0; i < size; i++ {
		s += fmt.Sprintf(" %c ", 'a'+i)
	}
	s += "\n"
	
	// Rows with row numbers
	for i := size - 1; i >= 0; i-- {
		s += fmt.Sprintf("%2d ", i+1)
		for j := 0; j < size; j++ {
			cell := m.game.Board[i][j]
			if cell == "" {
				s += " · "
			} else {
				s += fmt.Sprintf(" %s ", cell)
			}
		}
		s += fmt.Sprintf(" %d\n", i+1)
	}
	
	// Bottom column headers
	s += "   "
	for i := 0; i < size; i++ {
		s += fmt.Sprintf(" %c ", 'a'+i)
	}
	s += "\n"
	
	return s
}

// Commands
func (m model) startNewGame() tea.Cmd {
	return func() tea.Msg {
		// For now, create a simple game state
		// Later we'll make actual API calls
		board := make([][]string, m.boardSize)
		for i := range board {
			board[i] = make([]string, m.boardSize)
		}
		
		return gameStarted{
			game: &GameState{
				Board:  board,
				Status: "active",
				Turn:   1,
				Player: 1,
				Winner: 0,
			},
		}
	}
}

// Messages
type gameStarted struct {
	game *GameState
}