package ws

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WSManager struct {
	clients   map[*websocket.Conn]bool
	broadcast chan interface{}
	mu        sync.Mutex
}

func NewWSManager() *WSManager {
	return &WSManager{
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan interface{}),
	}
}

func (m *WSManager) HandleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("erro upgrade ws: %v", err)
		return
	}
	defer func() { _ = ws.Close() }()

	m.mu.Lock()
	m.clients[ws] = true
	m.mu.Unlock()

	for {
		_, _, err := ws.ReadMessage()
		if err != nil {
			m.mu.Lock()
			delete(m.clients, ws)
			m.mu.Unlock()
			break
		}
	}
}

func (m *WSManager) StartBroadcasting() {
	for {
		msg := <-m.broadcast
		m.mu.Lock()
		for client := range m.clients {
			err := client.WriteJSON(msg)
			if err != nil {
				_ = client.Close()
				delete(m.clients, client)
			}
		}
		m.mu.Unlock()
	}
}

func (m *WSManager) Notify(event interface{}) {
	m.broadcast <- event
}
