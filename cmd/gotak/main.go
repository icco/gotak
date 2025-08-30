package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

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
	boardSize  int
	difficulty string
	game       *GameState
	gameSlug   string
	
	// Board interaction state
	cursorX    int
	cursorY    int
	inputMode  inputMode
	moveInput  string
	
	// UI state
	width  int
	height int
	error  string
}

type inputMode int

const (
	inputModeNormal inputMode = iota
	inputModeMove
)

type GameState struct {
	Board   [][]Cell
	Status  string
	Turn    int
	Player  int
	Winner  int
	Moves   []string
}

type Cell struct {
	Stones []Stone
}

type Stone struct {
	Type   StoneType
	Player int
}

type StoneType int

const (
	StoneFlat StoneType = iota
	StoneStanding
	StoneCapstone
)

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
		m.gameSlug = msg.slug
		return m, nil
		
	case moveApplied:
		m.game = msg.game
		m.game.Moves = append(m.game.Moves, msg.move)
		m.inputMode = inputModeNormal
		m.moveInput = ""
		m.error = ""
		return m, nil
		
	case moveError:
		m.error = msg.error
		m.inputMode = inputModeNormal
		m.moveInput = ""
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
	if m.game == nil {
		switch msg.String() {
		case "q":
			m.screen = screenMenu
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}
	
	// Handle different input modes
	switch m.inputMode {
	case inputModeMove:
		return m.updateMoveInput(msg)
	case inputModeNormal:
		return m.updateNormalInput(msg)
	}
	
	return m, nil
}

func (m model) updateMoveInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = inputModeNormal
		m.moveInput = ""
		m.error = ""
		return m, nil
	case "enter":
		if m.moveInput != "" {
			return m, m.makeMove(m.moveInput)
		}
		return m, nil
	case "backspace":
		if len(m.moveInput) > 0 {
			m.moveInput = m.moveInput[:len(m.moveInput)-1]
		}
		return m, nil
	default:
		// Add character to move input
		if len(msg.String()) == 1 {
			m.moveInput += msg.String()
		}
		return m, nil
	}
}

func (m model) updateNormalInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.screen = screenMenu
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "m":
		// Enter move input mode
		m.inputMode = inputModeMove
		m.moveInput = ""
		m.error = ""
		return m, nil
	case "up", "k":
		if m.cursorY < len(m.game.Board)-1 {
			m.cursorY++
		}
		return m, nil
	case "down", "j":
		if m.cursorY > 0 {
			m.cursorY--
		}
		return m, nil
	case "left", "h":
		if m.cursorX > 0 {
			m.cursorX--
		}
		return m, nil
	case "right", "l":
		if m.cursorX < len(m.game.Board)-1 {
			m.cursorX++
		}
		return m, nil
	case "enter", " ":
		// Place flat stone at cursor position
		square := fmt.Sprintf("%c%d", 'a'+m.cursorX, m.cursorY+1)
		return m, m.makeMove(square)
	case "s":
		// Place standing stone at cursor position
		square := fmt.Sprintf("S%c%d", 'a'+m.cursorX, m.cursorY+1)
		return m, m.makeMove(square)
	case "c":
		// Place capstone at cursor position
		square := fmt.Sprintf("C%c%d", 'a'+m.cursorX, m.cursorY+1)
		return m, m.makeMove(square)
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
	
	// Show input mode and controls
	var controls string
	switch m.inputMode {
	case inputModeMove:
		controls = menuItemStyle.Render(fmt.Sprintf("Move Input: %s | Press Enter to submit, Esc to cancel", m.moveInput))
	case inputModeNormal:
		cursorPos := fmt.Sprintf("%c%d", 'a'+m.cursorX, m.cursorY+1)
		controls = menuItemStyle.Render(fmt.Sprintf("Cursor: %s | ↑↓←→/hjkl: move | Enter: flat | S: standing | C: capstone | M: manual input | Q: menu", cursorPos))
	}
	
	// Show recent moves
	moveHistory := ""
	if len(m.game.Moves) > 0 {
		recent := m.game.Moves
		if len(recent) > 5 {
			recent = recent[len(recent)-5:]
		}
		moveHistory = menuItemStyle.Render("Recent moves: " + fmt.Sprintf("%v", recent))
	}
	
	content := []string{title, "", gameInfo, "", board, "", controls}
	if moveHistory != "" {
		content = append(content, "", moveHistory)
	}
	
	if m.error != "" {
		errorMsg := errorStyle.Render(fmt.Sprintf("❌ Error: %s", m.error))
		content = append(content, "", errorMsg)
	}
	
	return lipgloss.JoinVertical(lipgloss.Left, content...)
}

func (m model) viewSettings() string {
	title := titleStyle.Render("⚙️ Settings")
	
	settings := []string{
		fmt.Sprintf("Board Size: %dx%d", m.boardSize, m.boardSize),
		fmt.Sprintf("AI Difficulty: %s", m.difficulty),
		fmt.Sprintf("Server: %s", m.serverURL),
	}
	
	settingsContent := ""
	for _, setting := range settings {
		settingsContent += menuItemStyle.Render(setting) + "\n"
	}
	
	help := menuItemStyle.Render("Press q to go back to menu")
	
	return lipgloss.JoinVertical(lipgloss.Left, title, "", settingsContent, help)
}

func (m model) renderBoard() string {
	if m.game == nil || m.game.Board == nil {
		return "No game board"
	}
	
	size := len(m.game.Board)
	var rows []string
	
	// Top column headers
	header := "   "
	for i := 0; i < size; i++ {
		header += fmt.Sprintf(" %c ", 'a'+i)
	}
	rows = append(rows, header)
	
	// Board rows (reverse order for proper display)
	for i := size - 1; i >= 0; i-- {
		row := fmt.Sprintf("%2d ", i+1)
		for j := 0; j < size; j++ {
			cell := m.game.Board[i][j]
			content := m.renderCell(cell, j, i)
			row += content
		}
		row += fmt.Sprintf(" %d", i+1)
		rows = append(rows, row)
	}
	
	// Bottom column headers
	footer := "   "
	for i := 0; i < size; i++ {
		footer += fmt.Sprintf(" %c ", 'a'+i)
	}
	rows = append(rows, footer)
	
	boardContent := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return boardStyle.Render(boardContent)
}

func (m model) renderCell(cell Cell, x, y int) string {
	isCursor := (x == m.cursorX && y == m.cursorY)
	
	var content string
	var bgColor, fgColor string
	
	if len(cell.Stones) == 0 {
		// Empty cell
		content = "·"
		bgColor = "235"
		fgColor = "240"
	} else {
		// Cell has stones - show the top stone
		topStone := cell.Stones[len(cell.Stones)-1]
		content = m.renderStone(topStone)
		
		// Color based on player
		if topStone.Player == 1 {
			bgColor = "240"
			fgColor = "255"  // White pieces
		} else {
			bgColor = "240"
			fgColor = "240"  // Black pieces
		}
	}
	
	// Highlight cursor
	if isCursor {
		bgColor = "220"  // Yellow background for cursor
		fgColor = "16"   // Black text on yellow
	}
	
	return cellStyle.
		Background(lipgloss.Color(bgColor)).
		Foreground(lipgloss.Color(fgColor)).
		Render(content)
}

func (m model) renderStone(stone Stone) string {
	switch stone.Type {
	case StoneFlat:
		if stone.Player == 1 {
			return "○"  // White flat stone
		} else {
			return "●"  // Black flat stone
		}
	case StoneStanding:
		if stone.Player == 1 {
			return "□"  // White standing stone
		} else {
			return "■"  // Black standing stone
		}
	case StoneCapstone:
		if stone.Player == 1 {
			return "◇"  // White capstone
		} else {
			return "◆"  // Black capstone
		}
	default:
		return "?"
	}
}

// API types matching server
type APIGameResponse struct {
	ID     int64             `json:"id"`
	Slug   string            `json:"slug"`
	Status string            `json:"status"`
	Size   int               `json:"size"`
	Turns  []APITurn         `json:"turns"`
	Tags   map[string]string `json:"tags"`
}

type APITurn struct {
	Moves []APIMove `json:"moves"`
}

type APIMove struct {
	Player int    `json:"player"`
	Text   string `json:"text"`
}

type APIMoveRequest struct {
	Player int    `json:"player"`
	Move   string `json:"move"`
	Turn   int64  `json:"turn"`
}

// Commands
func (m model) startNewGame() tea.Cmd {
	return func() tea.Msg {
		// Try API first, fallback to local mode
		if game, slug, err := m.startGameViaAPI(); err == nil {
			return gameStarted{game: game, slug: slug}
		}
		
		// Fallback to local mode for testing
		game := m.startLocalGame()
		return gameStarted{game: game, slug: "local-game"}
	}
}

func (m model) startGameViaAPI() (*GameState, string, error) {
	payload := map[string]interface{}{
		"size": m.boardSize,
	}
	
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}

	resp, err := http.Post(m.serverURL+"/game/new", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusTemporaryRedirect {
		return nil, "", fmt.Errorf("server error: %d", resp.StatusCode)
	}

	var gameResp APIGameResponse
	if err := json.NewDecoder(resp.Body).Decode(&gameResp); err != nil {
		return nil, "", err
	}

	game := m.convertAPIGameToState(gameResp)
	return game, gameResp.Slug, nil
}

func (m model) startLocalGame() *GameState {
	// Create empty board for local testing
	board := make([][]Cell, m.boardSize)
	for i := range board {
		board[i] = make([]Cell, m.boardSize)
		for j := range board[i] {
			board[i][j] = Cell{Stones: []Stone{}}
		}
	}
	
	return &GameState{
		Board:  board,
		Status: "active",
		Turn:   1,
		Player: 1,
		Winner: 0,
		Moves:  []string{},
	}
}

func (m model) makeMove(moveStr string) tea.Cmd {
	return func() tea.Msg {
		if m.game == nil {
			return moveError{error: "No active game"}
		}

		// If local game, process locally
		if m.gameSlug == "local-game" {
			return m.makeLocalMove(moveStr)
		}

		// Try API move
		if game, err := m.makeMoveViaAPI(moveStr); err == nil {
			return moveApplied{game: game, move: moveStr}
		} else {
			// Fallback to local for testing
			return m.makeLocalMove(moveStr)
		}
	}
}

func (m model) makeMoveViaAPI(moveStr string) (*GameState, error) {
	moveReq := APIMoveRequest{
		Player: m.game.Player,
		Move:   moveStr,
		Turn:   int64(m.game.Turn),
	}

	data, err := json.Marshal(moveReq)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(m.serverURL+"/game/"+m.gameSlug+"/move", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("move rejected: %d", resp.StatusCode)
	}

	var gameResp APIGameResponse
	if err := json.NewDecoder(resp.Body).Decode(&gameResp); err != nil {
		return nil, err
	}

	return m.convertAPIGameToState(gameResp), nil
}

func (m model) makeLocalMove(moveStr string) tea.Msg {
	// Simple local move validation and application
	if err := m.applyMoveToBoard(m.game.Board, moveStr, m.game.Player); err != nil {
		return moveError{error: err.Error()}
	}

	// Create new game state
	newGame := &GameState{
		Board:  make([][]Cell, len(m.game.Board)),
		Status: m.game.Status,
		Turn:   m.game.Turn + 1,
		Player: 3 - m.game.Player, // Switch between 1 and 2
		Winner: m.game.Winner,
		Moves:  make([]string, len(m.game.Moves)),
	}

	// Deep copy board
	for i := range m.game.Board {
		newGame.Board[i] = make([]Cell, len(m.game.Board[i]))
		for j := range m.game.Board[i] {
			newGame.Board[i][j] = Cell{
				Stones: make([]Stone, len(m.game.Board[i][j].Stones)),
			}
			copy(newGame.Board[i][j].Stones, m.game.Board[i][j].Stones)
		}
	}

	// Copy moves
	copy(newGame.Moves, m.game.Moves)

	return moveApplied{game: newGame, move: moveStr}
}

func (m model) convertAPIGameToState(apiGame APIGameResponse) *GameState {
	// Create empty board
	board := make([][]Cell, apiGame.Size)
	for i := range board {
		board[i] = make([]Cell, apiGame.Size)
		for j := range board[i] {
			board[i][j] = Cell{Stones: []Stone{}}
		}
	}
	
	// Extract moves from API turns
	var moves []string
	currentPlayer := 1
	
	// Process each turn and its moves
	for _, turn := range apiGame.Turns {
		for _, move := range turn.Moves {
			moves = append(moves, move.Text)
			
			// Parse and apply move to board (basic implementation)
			// This is a simplified version - in practice you'd use the actual gotak game logic
			if err := m.applyMoveToBoard(board, move.Text, move.Player); err != nil {
				// Handle error - for now just continue
				continue
			}
		}
	}
	
	// Determine current player based on move count
	if len(moves)%2 == 0 {
		currentPlayer = 1
	} else {
		currentPlayer = 2
	}
	
	return &GameState{
		Board:  board,
		Status: apiGame.Status,
		Turn:   len(moves) + 1,
		Player: currentPlayer,
		Winner: 0,
		Moves:  moves,
	}
}

func (m model) applyMoveToBoard(board [][]Cell, moveText string, player int) error {
	// Simple move parsing for placement moves only
	// This is a basic implementation - you'd want to use the full gotak parsing logic
	
	if len(moveText) < 2 {
		return fmt.Errorf("invalid move")
	}
	
	stoneType := StoneFlat
	i := 0
	
	// Check for stone type prefix
	switch moveText[0] {
	case 'S':
		stoneType = StoneStanding
		i = 1
	case 'C':
		stoneType = StoneCapstone
		i = 1
	case 'F':
		stoneType = StoneFlat
		i = 1
	}
	
	if i+1 >= len(moveText) {
		return fmt.Errorf("invalid move format")
	}
	
	// Parse column and row
	col := int(moveText[i] - 'a')
	row := int(moveText[i+1] - '1')
	
	// Check bounds
	if col < 0 || col >= len(board) || row < 0 || row >= len(board) {
		return fmt.Errorf("move out of bounds")
	}
	
	// Add stone to board
	stone := Stone{
		Type:   stoneType,
		Player: player,
	}
	board[row][col].Stones = append(board[row][col].Stones, stone)
	
	return nil
}

// Messages
type gameStarted struct {
	game *GameState
	slug string
}

type moveApplied struct {
	game *GameState
	move string
}

type moveError struct {
	error string
}