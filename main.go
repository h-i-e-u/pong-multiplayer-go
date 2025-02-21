package main

import (
	"flag"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Game dimensions and constants.
const (
	width  = 600 // 1.5 times the original width
	height = 450 // 1.5 times the original height
)

// GameState holds the positions and scores.
type GameState struct {
	BallX  float64 `json:"ballX"`
	BallY  float64 `json:"ballY"`
	BallVelX float64 `json:"ballVelX"`
	BallVelY float64 `json:"ballVelY"`
	Paddle1Y float64 `json:"paddle1Y"`
	Paddle2Y float64 `json:"paddle2Y"`
	Score1 int `json:"score1"`
	Score2 int `json:"score2"`
}

var (
	gameState GameState
	mu        sync.Mutex
	clients   = make(map[*websocket.Conn]bool)
	upgrader  = websocket.Upgrader{
		// Allow any origin for this demo (for production, restrict this)
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func main() {
	// Initialize game state
	gameState = GameState{
		BallX:    width / 2,
		BallY:    height / 2,
		BallVelX: 1.5, // Smoother velocity
		BallVelY: 1.5, // Smoother velocity
		Paddle1Y: height/2 - 37.5, // Adjusted for new height
		Paddle2Y: height/2 - 37.5, // Adjusted for new height
	}

	// Serve WebSocket endpoint and static files.
	http.HandleFunc("/ws", wsHandler)
	http.Handle("/", http.FileServer(http.Dir("./public")))

	// Run the game loop in a separate goroutine.
	go gameLoop()

	addr := flag.String("addr", ":8080", "http service address")
	flag.Parse()
	log.Println("Server starting at", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

// wsHandler upgrades HTTP connections to WebSocket and listens for paddle move messages.
func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer conn.Close()

	// Add new client
	mu.Lock()
	clients[conn] = true
	mu.Unlock()
	log.Println("New client connected:", conn.RemoteAddr())

	// Listen for messages (e.g., paddle movement)
	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			log.Println("Read error:", err)
			break
		}

		// Expected message format: { "type": "move", "paddle": 1 (or 2), "direction": "up" or "down" }
		if msg["type"] == "move" {
			paddle := int(msg["paddle"].(float64))
			direction := msg["direction"].(string)
			mu.Lock()
			if paddle == 1 {
				if direction == "up" {
					gameState.Paddle1Y -= 16
					if gameState.Paddle1Y < 0 {
						gameState.Paddle1Y = 0
					}
				} else if direction == "down" {
					gameState.Paddle1Y += 16
					if gameState.Paddle1Y > height-75 { // Adjusted for new paddle height
						gameState.Paddle1Y = height - 75 // Adjusted for new paddle height
					}
				}
			} else if paddle == 2 {
				if direction == "up" {
					gameState.Paddle2Y -= 16
					if gameState.Paddle2Y < 0 {
						gameState.Paddle2Y = 0
					}
				} else if direction == "down" {
					gameState.Paddle2Y += 16
					if gameState.Paddle2Y > height-75 { // Adjusted for new paddle height
						gameState.Paddle2Y = height - 75 // Adjusted for new paddle height
					}
				}
			}
			mu.Unlock()
		}
	}

	// Remove client when disconnected.
	mu.Lock()
	delete(clients, conn)
	mu.Unlock()
	log.Println("Client disconnected:", conn.RemoteAddr())
}

// gameLoop runs at roughly 60 frames per second.
func gameLoop() {
	ticker := time.NewTicker(16 * time.Millisecond) // ~60fps
	defer ticker.Stop()
	for range ticker.C {
		updateGame()
		broadcastGameState()
	}
}

// updateGame updates the ball position and handles collisions.
func updateGame() {
	mu.Lock()
	defer mu.Unlock()

	// Move ball.
	gameState.BallX += gameState.BallVelX
	gameState.BallY += gameState.BallVelY

	// Bounce off top and bottom.
	if gameState.BallY <= 0 || gameState.BallY >= height {
		gameState.BallVelY = -gameState.BallVelY
	}

	// Check left side (player 1 paddle).
	if gameState.BallX <= 30 { // Adjusted for new width
		if gameState.BallY >= gameState.Paddle1Y && gameState.BallY <= gameState.Paddle1Y+75 { // Adjusted for new paddle height
			gameState.BallVelX = -gameState.BallVelX
		} else {
			// Player 2 scores.
			gameState.Score2++
			resetBall()
		}
	}

	// Check right side (player 2 paddle).
	if gameState.BallX >= width-30 { // Adjusted for new width
		if gameState.BallY >= gameState.Paddle2Y && gameState.BallY <= gameState.Paddle2Y+75 { // Adjusted for new paddle height
			gameState.BallVelX = -gameState.BallVelX
		} else {
			// Player 1 scores.
			gameState.Score1++
			resetBall()
		}
	}
}

// resetBall centers the ball and randomizes its direction.
func resetBall() {
	gameState.BallX = width / 2
	gameState.BallY = height / 2
	if rand.Intn(2) == 0 {
		gameState.BallVelX = 1.5 // Smoother velocity
	} else {
		gameState.BallVelX = -1.5 // Smoother velocity
	}
	if rand.Intn(2) == 0 {
		gameState.BallVelY = 1.5 // Smoother velocity
	} else {
		gameState.BallVelY = -1.5 // Smoother velocity
	}
}

// broadcastGameState sends the current game state to all connected clients.
func broadcastGameState() {
	mu.Lock()
	state := gameState
	mu.Unlock()

	for client := range clients {
		if err := client.WriteJSON(state); err != nil {
			log.Println("Write error:", err)
			client.Close()
			mu.Lock()
			delete(clients, client)
			mu.Unlock()
		}
	}
}
