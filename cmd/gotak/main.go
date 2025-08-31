package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/icco/gotak"
)

func getVersion() string {
	if tag := os.Getenv("GIT_TAG"); tag != "" {
		return tag
	}
	return "dev"
}

var (
	localFlag = flag.Bool("local", false, "Use local server instead of https://gotak.app")

	// Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Align(lipgloss.Center).
			MarginBottom(2)

	menuItemStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	selectedMenuItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("170")).
				Bold(true).
				PaddingLeft(2)

	boardStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(2).
			Align(lipgloss.Center)


	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true).
			Align(lipgloss.Center).
			MarginTop(1)

	inputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1).
			MarginTop(1)
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
	)

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

type screen int

const (
	screenAuthMode screen = iota // Choose login or register
	screenAuth                   // Login/register form
	screenMenu
	screenGame
	screenSettings
)

type authMode int

const (
	authModeLogin authMode = iota
	authModeRegister
)

type model struct {
	serverURL string
	screen    screen

	// Auth state
	authMode       authMode
	authModeCursor int // For selecting login/register
	emailInput     textinput.Model
	passwordInput  textinput.Model
	nameInput      textinput.Model
	token          string
	authenticated  bool
	authFocus      int

	// Menu state
	menuCursor int

	// Game state
	gameSlug  string
	gameData  *GameData
	boardSize int

	// Move input
	moveInput string

	// UI state
	width     int
	height    int
	error     string
	spinner   spinner.Model
	isLoading bool
}

type GameData struct {
	ID     int64             `json:"id"`
	Slug   string            `json:"slug"`
	Status string            `json:"status"`
	Size   int               `json:"size"`
	Turns  []GameTurn        `json:"turns"`
	Tags   map[string]string `json:"tags"`
}

type GameTurn struct {
	Moves []GameMove `json:"moves"`
}

type GameMove struct {
	Player int    `json:"player"`
	Text   string `json:"text"`
}

func initialModel(serverURL string) model {
	// Initialize text inputs
	emailInput := textinput.New()
	emailInput.Placeholder = "Email address"
	emailInput.CharLimit = 320

	passwordInput := textinput.New()
	passwordInput.Placeholder = "Password"
	passwordInput.EchoMode = textinput.EchoPassword
	passwordInput.CharLimit = 128

	nameInput := textinput.New()
	nameInput.Placeholder = "Full name"
	nameInput.CharLimit = 128

	// Initialize spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return model{
		serverURL:     serverURL,
		screen:        screenAuthMode, // Start with mode selection
		authMode:      authModeLogin,
		emailInput:    emailInput,
		passwordInput: passwordInput,
		nameInput:     nameInput,
		spinner:       s,
		boardSize:     5,
		authenticated: false,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case authSuccess:
		m.token = msg.token
		m.authenticated = true
		m.screen = screenMenu
		m.error = ""
		m.isLoading = false
		return m, nil

	case registrationSuccess:
		// Clear form data and show success, then switch to login mode
		m.passwordInput.SetValue("")
		m.nameInput.SetValue("")
		m.authMode = authModeLogin
		m.authFocus = 0
		m.error = "Registration successful! Please login with your credentials."
		m.isLoading = false
		return m, nil

	case gameLoaded:
		m.gameData = msg.game
		m.gameSlug = msg.game.Slug
		m.screen = screenGame
		m.error = ""
		m.isLoading = false
		return m, nil

	case apiError:
		m.error = msg.error
		m.isLoading = false
		return m, nil

	case moveSubmitted:
		m.gameData = msg.game
		m.moveInput = ""
		m.error = ""
		m.isLoading = false
		return m, nil

	case tea.KeyMsg:
		switch m.screen {
		case screenAuthMode:
			return m.updateAuthMode(msg)
		case screenAuth:
			return m.updateAuth(msg)
		case screenMenu:
			return m.updateMenu(msg)
		case screenGame:
			return m.updateGame(msg)
		case screenSettings:
			return m.updateSettings(msg)
		}
	}

	// Update spinner
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m model) updateAuthMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m, tea.Quit
	case "up", "k":
		if m.authModeCursor > 0 {
			m.authModeCursor--
		}
		return m, nil
	case "down", "j":
		if m.authModeCursor < 1 { // 0: Login, 1: Register
			m.authModeCursor++
		}
		return m, nil
	case "enter":
		// Set the auth mode based on selection
		if m.authModeCursor == 0 {
			m.authMode = authModeLogin
		} else {
			m.authMode = authModeRegister
		}
		m.screen = screenAuth
		m.authFocus = 0 // Start at first field
		m.error = ""    // Clear any errors

		// Focus the email field
		m.emailInput.Focus()
		m.passwordInput.Blur()
		m.nameInput.Blur()

		return m, nil
	}
	return m, nil
}

func (m model) updateAuth(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		// Go back to auth mode selection
		m.screen = screenAuthMode
		m.error = ""
		return m, nil
	case "tab", "down":
		maxFields := 2 // email, password, submit button (login)
		if m.authMode == authModeRegister {
			maxFields = 3 // email, password, name, submit button (register)
		}

		// Blur current field
		switch m.authFocus {
		case 0:
			m.emailInput.Blur()
		case 1:
			m.passwordInput.Blur()
		case 2:
			m.nameInput.Blur()
		}

		if m.authFocus < maxFields {
			m.authFocus++
		} else {
			// Loop back to first field
			m.authFocus = 0
		}

		// Focus new field
		switch m.authFocus {
		case 0:
			m.emailInput.Focus()
		case 1:
			m.passwordInput.Focus()
		case 2:
			m.nameInput.Focus()
		}

		return m, nil
	case "up", "shift+tab":
		// Blur current field
		switch m.authFocus {
		case 0:
			m.emailInput.Blur()
		case 1:
			m.passwordInput.Blur()
		case 2:
			m.nameInput.Blur()
		}

		if m.authFocus > 0 {
			m.authFocus--
		} else {
			// Loop to last field
			maxFields := 2 // email, password, submit button (login)
			if m.authMode == authModeRegister {
				maxFields = 3 // email, password, name, submit button (register)
			}
			m.authFocus = maxFields
		}

		// Focus new field
		switch m.authFocus {
		case 0:
			m.emailInput.Focus()
		case 1:
			m.passwordInput.Focus()
		case 2:
			m.nameInput.Focus()
		}

		return m, nil
	case "enter":
		maxFocus := 1
		if m.authMode == authModeRegister {
			maxFocus = 2
		}

		// If on submit button or any field, validate and submit the form
		if m.authFocus == maxFocus+1 || m.authFocus <= maxFocus {
			// Get values from textinputs
			email := strings.TrimSpace(m.emailInput.Value())
			password := strings.TrimSpace(m.passwordInput.Value())
			name := strings.TrimSpace(m.nameInput.Value())

			// Basic validation
			if email == "" {
				m.error = "Email address is required"
				return m, nil
			}
			if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
				m.error = "Please enter a valid email address"
				return m, nil
			}
			if password == "" {
				m.error = "Password is required"
				return m, nil
			}
			if len(password) < 8 {
				m.error = "Password must be at least 8 characters long"
				return m, nil
			}
			if m.authMode == authModeRegister && name == "" {
				m.error = "Full name is required for registration"
				return m, nil
			}

			// Clear any previous errors
			m.error = ""

			// Start loading
			m.isLoading = true
			m.error = ""

			if m.authMode == authModeLogin {
				return m, m.loginUser()
			} else {
				return m, m.registerUser()
			}
		}
		return m, nil
	}

	// Handle input for the focused field
	switch m.authFocus {
	case 0:
		m.emailInput, cmd = m.emailInput.Update(msg)
	case 1:
		m.passwordInput, cmd = m.passwordInput.Update(msg)
	case 2:
		if m.authMode == authModeRegister {
			m.nameInput, cmd = m.nameInput.Update(msg)
		}
	}

	return m, cmd
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
		if m.menuCursor < 2 {
			m.menuCursor++
		}
	case "enter", " ":
		switch m.menuCursor {
		case 0: // New Game
			m.isLoading = true
			return m, m.createGame()
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
	case "enter":
		if m.moveInput != "" {
			m.isLoading = true
			return m, m.submitMove()
		}
		return m, nil
	case "backspace":
		if len(m.moveInput) > 0 {
			m.moveInput = m.moveInput[:len(m.moveInput)-1]
		}
		return m, nil
	default:
		if gotak.IsValidMoveCharacter(msg.String()) {
			m.moveInput += msg.String()
		}
		return m, nil
	}
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
	var content string

	switch m.screen {
	case screenAuthMode:
		content = m.viewAuthMode()
	case screenAuth:
		content = m.viewAuth()
	case screenMenu:
		content = m.viewMenu()
	case screenGame:
		content = m.viewGame()
	case screenSettings:
		content = m.viewSettings()
	default:
		content = "Unknown screen"
	}

	// Add spinner in bottom right if loading
	if m.isLoading {
		content = m.withSpinner(content)
	}

	return content
}

func (m model) withSpinner(content string) string {
	if m.width <= 0 || m.height <= 0 {
		return content
	}

	spinnerStr := m.spinner.View() + " Loading..."

	// Position spinner in bottom right
	spinnerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Align(lipgloss.Right).
		Width(m.width).
		Height(1)

	bottomLine := spinnerStyle.Render(spinnerStr)

	// Split content into lines and replace the last line with spinner
	lines := strings.Split(content, "\n")
	if len(lines) > 0 {
		lines[len(lines)-1] = bottomLine
	} else {
		lines = []string{bottomLine}
	}

	return strings.Join(lines, "\n")
}

func (m model) viewAuthMode() string {
	formWidth := 40
	cardPadding := 2

	// Card style with border and background
	cardStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Background(lipgloss.Color("235")).
		Padding(cardPadding).
		Width(formWidth)

	// Header style
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true).
		Align(lipgloss.Center).
		MarginBottom(1)

	// Option styles
	activeOptionStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("39")).
		Foreground(lipgloss.Color("15")).
		Bold(true).
		Padding(1, 2).
		MarginTop(1).
		Align(lipgloss.Center).
		Width(formWidth - cardPadding*2)

	inactiveOptionStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("240")).
		Foreground(lipgloss.Color("247")).
		Padding(1, 2).
		MarginTop(1).
		Align(lipgloss.Center).
		Width(formWidth - cardPadding*2)

	// Form content
	formContent := []string{
		headerStyle.Render("Welcome to GoTak"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Align(lipgloss.Center).Render("Please choose an option"),
		"", // spacer
	}

	// Login option
	var loginOption string
	if m.authModeCursor == 0 {
		loginOption = activeOptionStyle.Render("► Sign In")
	} else {
		loginOption = inactiveOptionStyle.Render("Sign In")
	}
	formContent = append(formContent, loginOption)

	// Register option
	var registerOption string
	if m.authModeCursor == 1 {
		registerOption = activeOptionStyle.Render("► Create Account")
	} else {
		registerOption = inactiveOptionStyle.Render("Create Account")
	}
	formContent = append(formContent, registerOption)

	// Instructions
	instructions := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Align(lipgloss.Center).
		MarginTop(2).
		Render("↑/↓: Navigate • Enter: Select • Esc: Quit")
	formContent = append(formContent, "", instructions)

	// Create the form card
	form := cardStyle.Render(strings.Join(formContent, "\n"))

	// Error message if any
	var content string
	if m.error != "" {
		errorCard := lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("196")).
			Background(lipgloss.Color("52")).
			Foreground(lipgloss.Color("15")).
			Padding(1).
			Width(formWidth).
			MarginBottom(2).
			Render("⚠ " + m.error)
		content = lipgloss.JoinVertical(lipgloss.Center, errorCard, form)
	} else {
		content = form
	}

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) viewAuth() string {
	formWidth := 50
	cardPadding := 2

	// Card style with border and background
	cardStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Background(lipgloss.Color("235")).
		Padding(cardPadding).
		Width(formWidth)

	// Header style
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true).
		Align(lipgloss.Center).
		MarginBottom(1)

	// Input field styles
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("247")).
		MarginBottom(0)

	// Button styles
	activeButtonStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("39")).
		Foreground(lipgloss.Color("15")).
		Bold(true).
		Padding(0, 2).
		MarginTop(1).
		Align(lipgloss.Center)

	inactiveButtonStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("240")).
		Foreground(lipgloss.Color("247")).
		Padding(0, 2).
		MarginTop(1).
		Align(lipgloss.Center)

	// Form title
	var title, subtitle string
	if m.authMode == authModeLogin {
		title = "Welcome Back"
		subtitle = "Sign in to your account"
	} else {
		title = "Create Account"
		subtitle = "Join GoTak to start playing"
	}

	formContent := []string{
		headerStyle.Render(title),
		lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Align(lipgloss.Center).Render(subtitle),
		"", // spacer
	}

	// Email field
	emailLabel := labelStyle.Render("Email Address")
	formContent = append(formContent, emailLabel, m.emailInput.View(), "")

	// Password field
	passwordLabel := labelStyle.Render("Password")
	formContent = append(formContent, passwordLabel, m.passwordInput.View(), "")

	// Name field for registration
	if m.authMode == authModeRegister {
		nameLabel := labelStyle.Render("Full Name")
		formContent = append(formContent, nameLabel, m.nameInput.View(), "")
	}

	// Submit button
	buttonText := "Sign In"
	if m.authMode == authModeRegister {
		buttonText = "Create Account"
	}

	maxFocus := 1
	if m.authMode == authModeRegister {
		maxFocus = 2
	}

	var submitButton string
	if m.authFocus == maxFocus+1 {
		submitButton = activeButtonStyle.Width(formWidth - cardPadding*2).Render("► " + buttonText)
	} else {
		submitButton = inactiveButtonStyle.Width(formWidth - cardPadding*2).Render(buttonText)
	}
	formContent = append(formContent, submitButton)

	// Instructions
	instructions := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Align(lipgloss.Center).
		MarginTop(1).
		Render("Tab/↑/↓: Navigate • Enter: Submit • Esc: Back • Ctrl+C: Quit")
	formContent = append(formContent, "", instructions)

	// Create the form card
	form := cardStyle.Render(strings.Join(formContent, "\n"))

	// Error message if any
	var content string
	if m.error != "" {
		errorCard := lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("196")).
			Background(lipgloss.Color("52")).
			Foreground(lipgloss.Color("15")).
			Padding(1).
			Width(formWidth).
			MarginBottom(2).
			Render("⚠ " + m.error)
		content = lipgloss.JoinVertical(lipgloss.Center, errorCard, form)
	} else {
		content = form
	}

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) viewMenu() string {
	title := titleStyle.Width(m.width).Render("🎯 GoTak - Main Menu")

	choices := []string{
		"🎮 New Game",
		"⚙️  Settings",
		"🚪 Quit",
	}

	var menu string
	for i, choice := range choices {
		if m.menuCursor == i {
			menu += selectedMenuItemStyle.Render(fmt.Sprintf("> %s", choice)) + "\n"
		} else {
			menu += menuItemStyle.Render(fmt.Sprintf("  %s", choice)) + "\n"
		}
	}

	info := menuItemStyle.Render(fmt.Sprintf("Server: %s", m.serverURL))
	help := menuItemStyle.Render("↑/↓: Navigate | Enter: Select | Q: Quit")

	content := lipgloss.JoinVertical(lipgloss.Center, title, menu, info, help)

	if m.error != "" {
		errorMsg := errorStyle.Width(m.width).Render("❌ " + m.error)
		content = lipgloss.JoinVertical(lipgloss.Center, content, errorMsg)
	}

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) viewGame() string {
	if m.gameData == nil {
		loading := titleStyle.Width(m.width).Render("Loading game...")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, loading)
	}

	title := titleStyle.Width(m.width).Render(fmt.Sprintf("🎯 Game: %s", m.gameData.Slug))

	// Create a much larger board that fills most of the screen
	board := m.renderLargeBoard()

	// Move input area
	inputArea := inputStyle.Width(60).Render(fmt.Sprintf("Move: %s", m.moveInput))

	// Game info
	gameInfo := menuItemStyle.Render(fmt.Sprintf("Status: %s | Moves: %d",
		m.gameData.Status, m.getTotalMoves()))

	// Help text with proper Tak move examples
	help := menuItemStyle.Render("Move Examples: a1 (flat) | Sa1 (standing) | Ca1 (capstone) | 3a1>21 (move 3 stones) | Q: Menu")

	content := lipgloss.JoinVertical(lipgloss.Center, title, board, inputArea, gameInfo, help)

	if m.error != "" {
		errorMsg := errorStyle.Width(m.width).Render("❌ " + m.error)
		content = lipgloss.JoinVertical(lipgloss.Center, content, errorMsg)
	}

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) renderLargeBoard() string {
	size := m.gameData.Size
	if size == 0 {
		size = 5
	}

	// Reconstruct the board state from game moves
	board := m.reconstructBoardState()

	var s strings.Builder
	
	// Top border - based on gambit's approach
	s.WriteString(m.buildTopBorder(size))

	// Board rows (reverse order for proper display - row 1 at bottom)
	for i := size - 1; i >= 0; i-- {
		// Row number
		s.WriteString(fmt.Sprintf(" %d │", i+1))
		
		for j := 0; j < size; j++ {
			square := fmt.Sprintf("%c%d", 'a'+j, i+1)
			stones := board[square]
			
			var display string
			if len(stones) == 0 {
				display = " · "
			} else {
				// Show the top stone
				topStone := stones[len(stones)-1]
				symbol := m.getStoneSymbol(topStone)
				
				if len(stones) == 1 {
					display = fmt.Sprintf(" %s ", symbol)
				} else if len(stones) <= 9 {
					// Show stack count with top stone for small stacks
					display = fmt.Sprintf("%d%s ", len(stones), symbol)
				} else {
					// For large stacks, just show count
					display = fmt.Sprintf("%d+ ", len(stones))
				}
			}
			
			s.WriteString(display + "│")
		}
		
		s.WriteString(fmt.Sprintf(" %d\n", i+1))
		
		// Add row separator (except for last row)
		if i > 0 {
			s.WriteString(m.buildMiddleBorder(size))
		}
	}

	// Bottom border
	s.WriteString(m.buildBottomBorder(size))
	
	// Bottom column labels
	s.WriteString(m.buildColumnLabels(size))

	return boardStyle.Render(s.String())
}

// buildTopBorder creates the top border of the board
func (m model) buildTopBorder(size int) string {
	border := "   ┌"
	for i := 0; i < size; i++ {
		border += "───"
		if i < size-1 {
			border += "┬"
		}
	}
	border += "┐\n"
	return border
}

// buildMiddleBorder creates the middle separator of the board
func (m model) buildMiddleBorder(size int) string {
	border := "   ├"
	for i := 0; i < size; i++ {
		border += "───"
		if i < size-1 {
			border += "┼"
		}
	}
	border += "┤\n"
	return border
}

// buildBottomBorder creates the bottom border of the board
func (m model) buildBottomBorder(size int) string {
	border := "   └"
	for i := 0; i < size; i++ {
		border += "───"
		if i < size-1 {
			border += "┴"
		}
	}
	border += "┘\n"
	return border
}

// buildColumnLabels creates the column labels at the bottom
func (m model) buildColumnLabels(size int) string {
	labels := "     "
	for i := 0; i < size; i++ {
		labels += fmt.Sprintf("%c  ", 'a'+i)
	}
	return labels + "\n"
}

// reconstructBoardState recreates the board state by replaying all moves from the game data
func (m model) reconstructBoardState() map[string][]*gotak.Stone {
	if m.gameData == nil {
		return make(map[string][]*gotak.Stone)
	}

	size := int64(m.gameData.Size)
	if size == 0 {
		size = 5
	}

	// Create a new board
	board := &gotak.Board{Size: size}
	board.Init()

	// Replay all moves in order
	for turnIndex, turn := range m.gameData.Turns {
		for _, gameMove := range turn.Moves {
			// Parse the move from PTN text
			move, err := gotak.NewMove(gameMove.Text)
			if err != nil {
				continue // Skip invalid moves
			}

			// Apply the move to the board
			// For the first turn, white places black's stone (special Tak rule)
			player := gameMove.Player
			if turnIndex == 0 && len(m.gameData.Turns) > 0 && len(m.gameData.Turns[0].Moves) > 0 {
				// First move of the game: white places opponent's stone
				if player == gotak.PlayerWhite {
					player = gotak.PlayerBlack
				}
			}

			err = board.DoMove(move, player)
			if err != nil {
				// Skip invalid moves but could log for debugging
				continue
			}
		}
	}

	return board.Squares
}


// getStoneSymbol returns a visual symbol for a stone
func (m model) getStoneSymbol(stone *gotak.Stone) string {
	var playerSymbol string
	if stone.Player == gotak.PlayerWhite {
		playerSymbol = "○" // White circle
	} else {
		playerSymbol = "●" // Black circle
	}

	switch stone.Type {
	case gotak.StoneFlat:
		return playerSymbol
	case gotak.StoneStanding:
		if stone.Player == gotak.PlayerWhite {
			return "□" // White square for standing
		} else {
			return "■" // Black square for standing
		}
	case gotak.StoneCap:
		if stone.Player == gotak.PlayerWhite {
			return "◇" // White diamond for capstone
		} else {
			return "◆" // Black diamond for capstone
		}
	default:
		return playerSymbol
	}
}

func (m model) getTotalMoves() int {
	total := 0
	for _, turn := range m.gameData.Turns {
		total += len(turn.Moves)
	}
	return total
}

// getCurrentPlayer determines which player's turn it is based on move count
// Player 1 (White) goes first, Player 2 (Black) goes second, alternating
func (m model) getCurrentPlayer() int {
	if m.gameData == nil {
		return 1 // Default to player 1 if no game data
	}

	totalMoves := m.getTotalMoves()
	// Player 1 starts, so on even total moves it's player 1's turn
	// On odd total moves it's player 2's turn
	if totalMoves%2 == 0 {
		return 1
	}
	return 2
}

func (m model) viewSettings() string {
	title := titleStyle.Width(m.width).Render("⚙️ Settings")

	settings := []string{
		fmt.Sprintf("Board Size: %dx%d", m.boardSize, m.boardSize),
		fmt.Sprintf("Server: %s", m.serverURL),
	}

	settingsContent := ""
	for _, setting := range settings {
		settingsContent += menuItemStyle.Render(setting) + "\n"
	}

	help := menuItemStyle.Render("Q: Back to menu")

	content := lipgloss.JoinVertical(lipgloss.Center, title, settingsContent, help)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// API Commands
func (m model) loginUser() tea.Cmd {
	return func() tea.Msg {
		payload := map[string]string{
			"email":    m.emailInput.Value(),
			"password": m.passwordInput.Value(),
		}

		data, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", m.serverURL+"/auth/login", bytes.NewBuffer(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", fmt.Sprintf("gotak-cli %s", getVersion()))

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return apiError{error: fmt.Sprintf("Connection failed: %v", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			// Read the actual error message from server
			var errorResp struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&errorResp); err == nil {
				return apiError{error: fmt.Sprintf("Login failed: %s", errorResp.Error)}
			}
			return apiError{error: fmt.Sprintf("Login failed (status %d)", resp.StatusCode)}
		}

		var authResp struct {
			Token string `json:"token"`
			User  struct {
				ID    int64  `json:"id"`
				Email string `json:"email"`
				Name  string `json:"name"`
			} `json:"user"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
			return apiError{error: "Login response error"}
		}

		return authSuccess{token: authResp.Token}
	}
}

func (m model) registerUser() tea.Cmd {
	return func() tea.Msg {
		payload := map[string]string{
			"email":    m.emailInput.Value(),
			"password": m.passwordInput.Value(),
			"name":     m.nameInput.Value(),
		}

		data, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", m.serverURL+"/auth/register", bytes.NewBuffer(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", fmt.Sprintf("gotak-cli %s", getVersion()))

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return apiError{error: fmt.Sprintf("Connection failed: %v", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated { // Registration returns 201 on success
			// Read the actual error message from server
			var errorResp struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&errorResp); err == nil {
				return apiError{error: fmt.Sprintf("Registration failed: %s", errorResp.Error)}
			}
			return apiError{error: fmt.Sprintf("Registration failed (status %d)", resp.StatusCode)}
		}

		// Registration successful, now show success message and return to login
		return registrationSuccess{}
	}
}

func (m model) createGame() tea.Cmd {
	return func() tea.Msg {
		payload := map[string]interface{}{
			"size": m.boardSize,
		}

		data, _ := json.Marshal(payload)

		req, _ := http.NewRequest("POST", m.serverURL+"/game/new", bytes.NewBuffer(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+m.token)
		req.Header.Set("User-Agent", fmt.Sprintf("gotak-cli %s", getVersion()))

		// Don't follow redirects automatically
		client := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			return apiError{error: fmt.Sprintf("Connection failed: %v", err)}
		}
		defer resp.Body.Close()

		// Handle redirect response
		if resp.StatusCode == http.StatusTemporaryRedirect {
			// Extract game slug from Location header
			location := resp.Header.Get("Location")
			if location == "" {
				return apiError{error: "Game creation redirect missing location"}
			}

			// Extract slug from "/game/{slug}"
			parts := strings.Split(location, "/")
			if len(parts) < 3 || parts[1] != "game" {
				return apiError{error: "Invalid game location format"}
			}
			gameSlug := parts[2]

			// Now fetch the game data with a GET request
			getReq, _ := http.NewRequest("GET", m.serverURL+"/game/"+gameSlug, nil)
			getReq.Header.Set("Authorization", "Bearer "+m.token)
			getReq.Header.Set("User-Agent", fmt.Sprintf("gotak-cli %s", getVersion()))

			getResp, err := client.Do(getReq)
			if err != nil {
				return apiError{error: fmt.Sprintf("Failed to fetch created game: %v", err)}
			}
			defer getResp.Body.Close()

			if getResp.StatusCode != http.StatusOK {
				return apiError{error: fmt.Sprintf("Failed to fetch game data (status %d)", getResp.StatusCode)}
			}

			var game GameData
			if err := json.NewDecoder(getResp.Body).Decode(&game); err != nil {
				return apiError{error: "Game data parsing error"}
			}

			return gameLoaded{game: &game}
		}

		if resp.StatusCode != http.StatusOK {
			// Read the actual error message from server
			var errorResp struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&errorResp); err == nil {
				return apiError{error: fmt.Sprintf("Create game failed: %s", errorResp.Error)}
			}
			return apiError{error: fmt.Sprintf("Create game failed (status %d)", resp.StatusCode)}
		}

		return apiError{error: "Unexpected response from server"}
	}
}

func (m model) submitMove() tea.Cmd {
	return func() tea.Msg {
		payload := map[string]interface{}{
			"player": m.getCurrentPlayer(),
			"move":   m.moveInput,
			"turn":   int64(m.getTotalMoves() + 1),
		}

		data, _ := json.Marshal(payload)

		req, _ := http.NewRequest("POST", m.serverURL+"/game/"+m.gameSlug+"/move", bytes.NewBuffer(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+m.token)
		req.Header.Set("User-Agent", fmt.Sprintf("gotak-cli %s", getVersion()))

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return apiError{error: fmt.Sprintf("Move failed: %v", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			// Read the actual error message from server
			var errorResp struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&errorResp); err == nil {
				return apiError{error: fmt.Sprintf("Move failed: %s", errorResp.Error)}
			}
			return apiError{error: fmt.Sprintf("Move failed (status %d)", resp.StatusCode)}
		}

		var game GameData
		if err := json.NewDecoder(resp.Body).Decode(&game); err != nil {
			return apiError{error: "Move response error"}
		}

		// Clear the move input on successful move
		return moveSubmitted{game: &game}
	}
}

// Messages
type authSuccess struct {
	token string
}

type registrationSuccess struct{}

type gameLoaded struct {
	game *GameData
}

type moveSubmitted struct {
	game *GameData
}

type apiError struct {
	error string
}
