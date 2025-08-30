package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	serverURL = "http://localhost:8080"
)

type GameResponse struct {
	ID     int64  `json:"id"`
	Slug   string `json:"slug"`
	Status string `json:"status"`
}

type MoveRequest struct {
	Move string `json:"move"`
}

type AIMoveRequest struct {
	Level     string `json:"level"`
	Style     string `json:"style"`
	TimeLimit string `json:"time_limit"`
}

type AIMoveResponse struct {
	Move string `json:"move"`
	Hint string `json:"hint,omitempty"`
}

func main() {
	fmt.Println("🎯 Welcome to GoTak!")
	fmt.Println("A Tak game implementation with AI opponents")
	fmt.Println()

	// Start server in background
	fmt.Println("🚀 Starting local server...")
	serverCmd := exec.Command("go", "run", "./server")
	err := serverCmd.Start()
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer serverCmd.Process.Kill()

	// Wait for server to start
	fmt.Println("⏳ Waiting for server to start...")
	time.Sleep(3 * time.Second)

	// Test server connection
	if !isServerReady() {
		log.Fatalf("Server failed to start properly")
	}

	// Get game parameters
	size := getBoardSize()
	difficulty := getDifficulty()

	// Create game via API
	gameSlug, err := createGame(size)
	if err != nil {
		log.Fatalf("Failed to create game: %v", err)
	}

	fmt.Printf("🎮 Starting %dx%d game against %s AI (Game: %s)\n", size, size, getDifficultyName(difficulty), gameSlug)
	fmt.Println("💡 Type 'help' for commands, 'quit' to exit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	gameOver := false

	for !gameOver {
		// Show current game state
		showGameState(gameSlug)

		// Human turn
		fmt.Print("Your move: ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())

		switch input {
		case "quit", "exit":
			fmt.Println("Thanks for playing!")
			return
		case "help", "h":
			showHelp()
			continue
		case "status":
			showGameState(gameSlug)
			continue
		case "":
			continue
		}

		// Make human move via API
		err := makeMove(gameSlug, input)
		if err != nil {
			fmt.Printf("❌ Invalid move: %v\nTry again.\n", err)
			continue
		}

		// AI turn
		fmt.Print("🤖 AI thinking...")

		aiMove, err := getAIMove(gameSlug, difficulty)
		if err != nil {
			fmt.Printf("\n❌ AI error: %v\n", err)
			// Game might be over
			showGameState(gameSlug)
			break
		}

		fmt.Printf(" AI plays: %s\n", aiMove)

		err = makeMove(gameSlug, aiMove)
		if err != nil {
			fmt.Printf("❌ Failed to make AI move: %v\n", err)
			break
		}
	}

	fmt.Println("\nThanks for playing GoTak! 🎯")
}

// API Helper Functions

func isServerReady() bool {
	for i := 0; i < 10; i++ {
		resp, err := http.Get(serverURL + "/healthz")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func createGame(size int) (string, error) {
	payload := map[string]interface{}{
		"size": size,
	}
	
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := http.Post(serverURL+"/game/new", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return "", fmt.Errorf("server error: %d", resp.StatusCode)
	}

	var game GameResponse
	err = json.NewDecoder(resp.Body).Decode(&game)
	if err != nil {
		return "", err
	}

	return game.Slug, nil
}

func makeMove(gameSlug, move string) error {
	payload := MoveRequest{Move: move}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(serverURL+"/game/"+gameSlug+"/move", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("move rejected by server: %d", resp.StatusCode)
	}

	return nil
}

func getAIMove(gameSlug string, difficulty DifficultyLevel) (string, error) {
	payload := AIMoveRequest{
		Level:     getDifficultyName(difficulty),
		Style:     "balanced", 
		TimeLimit: "10s",
	}
	
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := http.Post(serverURL+"/game/"+gameSlug+"/ai-move", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("AI request failed: %d", resp.StatusCode)
	}

	var aiResp AIMoveResponse
	err = json.NewDecoder(resp.Body).Decode(&aiResp)
	if err != nil {
		return "", err
	}

	return aiResp.Move, nil
}

func showGameState(gameSlug string) {
	resp, err := http.Get(serverURL + "/game/" + gameSlug)
	if err != nil {
		fmt.Printf("❌ Failed to get game state: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("❌ Server error getting game: %d\n", resp.StatusCode)
		return
	}

	// For now, just show that we got the state
	fmt.Println("📋 Game state updated")
}

// UI Helper Functions

type DifficultyLevel int

const (
	Beginner DifficultyLevel = iota
	Intermediate
	Advanced
	Expert
)

func getBoardSize() int {
	fmt.Println("📏 Board Size Selection:")
	fmt.Println("1. 4x4 (Quick)")
	fmt.Println("2. 5x5 (Standard)")
	fmt.Println("3. 6x6 (Extended)")
	fmt.Print("Select [1-3, default: 2]: ")

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		switch input {
		case "1":
			return 4
		case "3":
			return 6
		default:
			return 5
		}
	}
	return 5
}

func getDifficulty() DifficultyLevel {
	fmt.Println("\n🤖 AI Difficulty Selection:")
	fmt.Println("1. Beginner (Random moves)")
	fmt.Println("2. Intermediate (Minimax depth 3)")
	fmt.Println("3. Advanced (Minimax depth 5)")
	fmt.Println("4. Expert (Monte Carlo Tree Search)")
	fmt.Print("Select [1-4, default: 2]: ")

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		switch input {
		case "1":
			return Beginner
		case "3":
			return Advanced
		case "4":
			return Expert
		default:
			return Intermediate
		}
	}
	return Intermediate
}

func getDifficultyName(d DifficultyLevel) string {
	switch d {
	case Beginner:
		return "beginner"
	case Intermediate:
		return "intermediate"
	case Advanced:
		return "advanced"
	case Expert:
		return "expert"
	default:
		return "intermediate"
	}
}

func showHelp() {
	fmt.Println("\n📖 GoTak Commands:")
	fmt.Println("  help    - Show this help")
	fmt.Println("  status  - Show current game state")
	fmt.Println("  quit    - Exit the game")
	fmt.Println("\n📝 Move Format:")
	fmt.Println("  a1      - Place flat stone at a1")
	fmt.Println("  Sa1     - Place standing stone at a1")
	fmt.Println("  Ca1     - Place capstone at a1")
	fmt.Println("  3a1>21  - Move 3 stones from a1 right, dropping 2,1")
	fmt.Println()
}