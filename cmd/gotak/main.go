package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/icco/gotak"
	"github.com/icco/gotak/ai"
)

func main() {
	fmt.Println("🎯 Welcome to GoTak!")
	fmt.Println("A Tak game implementation with AI opponents")
	fmt.Println()

	// Get game parameters
	size := getBoardSize()
	difficulty := getDifficulty()

	// Create a new game
	game, err := gotak.NewGame(size, 1, "cli-game")
	if err != nil {
		log.Fatalf("Failed to create game: %v", err)
	}

	// Create AI engine
	engine := &ai.TakticianEngine{}
	aiConfig := ai.AIConfig{
		Level:     difficulty,
		Style:     ai.Balanced,
		TimeLimit: 10 * time.Second,
	}

	fmt.Printf("\n🎮 Starting a %dx%d game against %s AI\n", size, size, difficultyName(difficulty))
	fmt.Println("You are White, AI is Black")
	fmt.Println("Enter moves in PTN notation (e.g., 'a1', 'Ca1', 'Sa1', '3a3+3')")
	fmt.Println("Type 'help' for commands, 'quit' to exit")
	fmt.Println()

	game.PrintCurrentState()

	scanner := bufio.NewScanner(os.Stdin)
	gameOver := false

	for !gameOver {
		// Human turn (White)
		fmt.Print("Your move: ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())

		switch input {
		case "quit", "exit":
			fmt.Println("Thanks for playing!")
			return
		case "help":
			showHelp()
			continue
		case "board":
			game.PrintCurrentState()
			continue
		case "":
			continue
		}

		// Try to make human move
		if err := makeMove(game, input); err != nil {
			fmt.Printf("❌ Invalid move: %v\nTry again.\n", err)
			continue
		}

		game.PrintCurrentState()

		// Check if game is over
		if winner, over := game.GameOver(); over {
			gameOver = true
			if winner == 0 {
				fmt.Println("🤝 It's a tie!")
			} else if winner == int(gotak.PlayerWhite) {
				fmt.Println("🎉 You win!")
			} else {
				fmt.Println("💻 AI wins!")
			}
			break
		}

		// AI turn (Black)
		fmt.Print("🤖 AI thinking...")

		ctx := context.Background()
		aiMove, err := engine.GetMove(ctx, game, aiConfig)
		if err != nil {
			fmt.Printf("\n❌ AI error: %v\n", err)
			continue
		}

		fmt.Printf(" AI plays: %s\n", aiMove)

		if err := makeMove(game, aiMove); err != nil {
			fmt.Printf("❌ AI made invalid move %s: %v\n", aiMove, err)
			continue
		}

		game.PrintCurrentState()

		// Check if game is over after AI move
		if winner, over := game.GameOver(); over {
			gameOver = true
			if winner == 0 {
				fmt.Println("🤝 It's a tie!")
			} else if winner == int(gotak.PlayerWhite) {
				fmt.Println("🎉 You win!")
			} else {
				fmt.Println("💻 AI wins!")
			}
		}
	}
}

func getBoardSize() int64 {
	fmt.Print("Choose board size (4-8) [default: 5]: ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			return 5
		}
		if size, err := strconv.ParseInt(input, 10, 64); err == nil {
			if size >= 4 && size <= 8 {
				return size
			}
		}
	}
	fmt.Println("Invalid size, using 5x5")
	return 5
}

func getDifficulty() ai.DifficultyLevel {
	fmt.Println("\nChoose AI difficulty:")
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
			return ai.Beginner
		case "", "2":
			return ai.Intermediate
		case "3":
			return ai.Advanced
		case "4":
			return ai.Expert
		}
	}
	return ai.Intermediate
}

func difficultyName(level ai.DifficultyLevel) string {
	switch level {
	case ai.Beginner:
		return "Beginner"
	case ai.Intermediate:
		return "Intermediate"
	case ai.Advanced:
		return "Advanced"
	case ai.Expert:
		return "Expert"
	default:
		return "Unknown"
	}
}

func makeMove(game *gotak.Game, moveStr string) error {
	// Create a simple turn with the move
	move, err := gotak.NewMove(moveStr)
	if err != nil {
		return err
	}

	// Determine current player based on turn count
	// In Tak, White goes first, then alternates
	player := gotak.PlayerWhite
	if len(game.Turns) > 0 {
		lastTurn := game.Turns[len(game.Turns)-1]
		if lastTurn.Second == nil {
			player = gotak.PlayerBlack
		}
	}

	// Apply the move to the board
	return game.Board.DoMove(move, player)
}

func showHelp() {
	fmt.Println("\n📖 GoTak Commands:")
	fmt.Println("  help    - Show this help")
	fmt.Println("  board   - Show current board")
	fmt.Println("  quit    - Exit the game")
	fmt.Println("\n📋 Move Notation (PTN):")
	fmt.Println("  a1      - Place flat stone at a1")
	fmt.Println("  Sa1     - Place standing stone at a1")
	fmt.Println("  Ca1     - Place capstone at a1")
	fmt.Println("  3a3+3   - Move 3 stones from a3 up, dropping all 3")
	fmt.Println("  4a4>121 - Move 4 stones from a4 right, dropping 1,2,1")
	fmt.Println("  Directions: + (up), - (down), > (right), < (left)")
	fmt.Println()
}
