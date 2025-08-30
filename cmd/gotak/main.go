package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/icco/gotak"
)

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

	cellStyle = lipgloss.NewStyle().
			Width(4).
			Height(2).
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
	email          string
	password       string
	name           string
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
	width  int
	height int
	error  string
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
	return model{
		serverURL:     serverURL,
		screen:        screenAuthMode, // Start with mode selection
		authMode:      authModeLogin,
		boardSize:     5,
		authenticated: false,
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

	case authSuccess:
		m.token = msg.token
		m.authenticated = true
		m.screen = screenMenu
		m.error = ""
		return m, nil

	case gameLoaded:
		m.gameData = msg.game
		m.gameSlug = msg.game.Slug
		m.screen = screenGame
		m.error = ""
		return m, nil

	case apiError:
		m.error = msg.error
		return m, nil

	case moveSubmitted:
		m.gameData = msg.game
		m.moveInput = ""
		m.error = ""
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

	return m, nil
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
		return m, nil
	}
	return m, nil
}

func (m model) updateAuth(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		if m.authFocus < maxFields {
			m.authFocus++
		} else {
			// Loop back to first field
			m.authFocus = 0
		}
		return m, nil
	case "up", "shift+tab":
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
		return m, nil
	case "enter":
		maxFocus := 1
		if m.authMode == authModeRegister {
			maxFocus = 2
		}

		// If on submit button or any field, validate and submit the form
		if m.authFocus == maxFocus+1 || m.authFocus <= maxFocus {
			// Basic validation
			if strings.TrimSpace(m.email) == "" {
				m.error = "Email address is required"
				return m, nil
			}
			if !strings.Contains(m.email, "@") || !strings.Contains(m.email, ".") {
				m.error = "Please enter a valid email address"
				return m, nil
			}
			if strings.TrimSpace(m.password) == "" {
				m.error = "Password is required"
				return m, nil
			}
			if len(m.password) < 8 {
				m.error = "Password must be at least 8 characters long"
				return m, nil
			}
			if m.authMode == authModeRegister && strings.TrimSpace(m.name) == "" {
				m.error = "Full name is required for registration"
				return m, nil
			}

			// Clear any previous errors
			m.error = ""

			if m.authMode == authModeLogin {
				return m, m.loginUser()
			} else {
				return m, m.registerUser()
			}
		}
		return m, nil
	case "backspace":
		switch m.authFocus {
		case 0:
			if len(m.email) > 0 {
				m.email = m.email[:len(m.email)-1]
			}
		case 1:
			if len(m.password) > 0 {
				m.password = m.password[:len(m.password)-1]
			}
		case 2:
			if len(m.name) > 0 {
				m.name = m.name[:len(m.name)-1]
			}
		}
		return m, nil
	default:
		if len(msg.String()) == 1 {
			switch m.authFocus {
			case 0:
				m.email += msg.String()
			case 1:
				m.password += msg.String()
			case 2:
				m.name += msg.String()
			}
		}
		return m, nil
	}
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
	switch m.screen {
	case screenAuthMode:
		return m.viewAuthMode()
	case screenAuth:
		return m.viewAuth()
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

	activeInputStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("39")).
		Background(lipgloss.Color("237")).
		Padding(0, 1).
		Width(formWidth - cardPadding*2 - 2)

	inactiveInputStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Background(lipgloss.Color("236")).
		Padding(0, 1).
		Width(formWidth - cardPadding*2 - 2)

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
	var emailInput string
	if m.authFocus == 0 {
		emailInput = activeInputStyle.Render(m.email + "█")
	} else {
		emailInput = inactiveInputStyle.Render(m.email)
	}
	formContent = append(formContent, emailLabel, emailInput, "")

	// Password field
	passwordLabel := labelStyle.Render("Password")
	passwordDisplay := strings.Repeat("•", len(m.password))
	if m.authFocus == 1 {
		passwordDisplay += "█"
	}
	var passwordInput string
	if m.authFocus == 1 {
		passwordInput = activeInputStyle.Render(passwordDisplay)
	} else {
		passwordInput = inactiveInputStyle.Render(passwordDisplay)
	}
	formContent = append(formContent, passwordLabel, passwordInput, "")

	// Name field for registration
	if m.authMode == authModeRegister {
		nameLabel := labelStyle.Render("Full Name")
		var nameInput string
		if m.authFocus == 2 {
			nameInput = activeInputStyle.Render(m.name + "█")
		} else {
			nameInput = inactiveInputStyle.Render(m.name)
		}
		formContent = append(formContent, nameLabel, nameInput, "")
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

	// Create a much larger board representation
	var rows []string

	// Column headers
	header := "    "
	for i := 0; i < size; i++ {
		header += fmt.Sprintf("  %c  ", 'a'+i)
	}
	rows = append(rows, header)

	// Board rows (reverse order for proper display)
	for i := size - 1; i >= 0; i-- {
		row := fmt.Sprintf("%2d  ", i+1)
		for j := 0; j < size; j++ {
			// For now, show empty cells - we'll populate from server data
			cell := cellStyle.
				Background(lipgloss.Color("235")).
				Foreground(lipgloss.Color("240")).
				Render("·")
			row += cell
		}
		row += fmt.Sprintf("  %d", i+1)
		rows = append(rows, row)
	}

	// Bottom column headers
	footer := "    "
	for i := 0; i < size; i++ {
		footer += fmt.Sprintf("  %c  ", 'a'+i)
	}
	rows = append(rows, footer)

	boardContent := strings.Join(rows, "\n")
	return boardStyle.Width(size*6 + 10).Render(boardContent)
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
			"email":    m.email,
			"password": m.password,
		}

		data, _ := json.Marshal(payload)
		resp, err := http.Post(m.serverURL+"/auth/login", "application/json", bytes.NewBuffer(data))
		if err != nil {
			return apiError{error: fmt.Sprintf("Connection failed: %v", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
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
			"email":    m.email,
			"password": m.password,
			"name":     m.name,
		}

		data, _ := json.Marshal(payload)
		resp, err := http.Post(m.serverURL+"/auth/register", "application/json", bytes.NewBuffer(data))
		if err != nil {
			return apiError{error: fmt.Sprintf("Connection failed: %v", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != 201 { // Registration returns 201 on success
			// Read the actual error message from server
			var errorResp struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&errorResp); err == nil {
				return apiError{error: fmt.Sprintf("Registration failed: %s", errorResp.Error)}
			}
			return apiError{error: fmt.Sprintf("Registration failed (status %d)", resp.StatusCode)}
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
			return apiError{error: "Registration response error"}
		}

		return authSuccess{token: authResp.Token}
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

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return apiError{error: fmt.Sprintf("Connection failed: %v", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 && resp.StatusCode != 307 {
			// Read the actual error message from server
			var errorResp struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&errorResp); err == nil {
				return apiError{error: fmt.Sprintf("Create game failed: %s", errorResp.Error)}
			}
			return apiError{error: fmt.Sprintf("Create game failed (status %d)", resp.StatusCode)}
		}

		var game GameData
		if err := json.NewDecoder(resp.Body).Decode(&game); err != nil {
			return apiError{error: "Game creation response error"}
		}

		return gameLoaded{game: &game}
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

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return apiError{error: fmt.Sprintf("Move failed: %v", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
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

type gameLoaded struct {
	game *GameData
}

type moveSubmitted struct {
	game *GameData
}

type apiError struct {
	error string
}
