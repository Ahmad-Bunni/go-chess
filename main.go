package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}
var gameChannels = make(map[string]map[*websocket.Conn]bool)
var broadcast = make(chan Message)

type Message struct {
	GameCode string `json:"gamecode"`
	Type     string `json:"type"`
	Move     *Move  `json:"move,omitempty"`
	Text     string `json:"text,omitempty"`
}

type Move struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func main() {
	r := chi.NewRouter()

	workDir, _ := os.Getwd()
	filesDir := filepath.Join(workDir, "static")
	FileServer(r, "/", http.Dir(filesDir))

	r.Get("/ws/{gameCode}", handleConnections)

	go handleMessages()

	log.Println("Server started at :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}

func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", http.StatusMovedPermanently).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	gameCode := chi.URLParam(r, "gameCode")
	if gameCode == "" {
		http.Error(w, "Game code is required", http.StatusBadRequest)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()

	if gameChannels[gameCode] == nil {
		gameChannels[gameCode] = make(map[*websocket.Conn]bool)
	}
	gameChannels[gameCode][ws] = true

	for {
		var msg Message
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("error: %v", err)
			delete(gameChannels[gameCode], ws)
			if len(gameChannels[gameCode]) == 0 {
				delete(gameChannels, gameCode)
			}
			break
		}
		msg.GameCode = gameCode
		broadcast <- msg
	}
}

func handleMessages() {
	for {
		msg := <-broadcast
		for client := range gameChannels[msg.GameCode] {
			err := client.WriteJSON(msg)
			if err != nil {
				log.Printf("error: %v", err)
				client.Close()
				delete(gameChannels[msg.GameCode], client)
				if len(gameChannels[msg.GameCode]) == 0 {
					delete(gameChannels, msg.GameCode)
				}
			}
		}
	}
}
