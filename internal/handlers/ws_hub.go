package handlers

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type WsMessage struct {
	Type  string     `json:"type"`
	Room  string     `json:"room,omitempty"`
	Lines []string   `json:"lines,omitempty"`
	Total int        `json:"total,omitempty"`
	Count int        `json:"count,omitempty"`
	Files []FileInfo `json:"files,omitempty"`
}

type WsClient struct {
	Send chan []byte
	Room string
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*WsClient]bool
	rooms   map[string]map[*WsClient]bool

	Register   chan *WsClient
	Unregister chan *WsClient
	Broadcast  chan *WsMessage
}

var GlobalHub = &Hub{
	clients:    make(map[*WsClient]bool),
	rooms:      make(map[string]map[*WsClient]bool),
	Register:   make(chan *WsClient, 64),
	Unregister: make(chan *WsClient, 64),
	Broadcast:  make(chan *WsMessage, 256),
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.Register:
			h.mu.Lock()
			h.clients[c] = true
			if c.Room != "" {
				if h.rooms[c.Room] == nil {
					h.rooms[c.Room] = make(map[*WsClient]bool)
				}
				h.rooms[c.Room][c] = true
			}
			h.mu.Unlock()

		case c := <-h.Unregister:
			h.mu.Lock()
			if h.clients[c] {
				delete(h.clients, c)
				if c.Room != "" {
					delete(h.rooms[c.Room], c)
				}
				close(c.Send)
			}
			h.mu.Unlock()

		case msg := <-h.Broadcast:
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			h.mu.RLock()
			if msg.Type == "files" {
				for c := range h.clients {
					select {
					case c.Send <- data:
					default:
					}
				}
			} else if msg.Room != "" {
				for c := range h.rooms[msg.Room] {
					select {
					case c.Send <- data:
					default:
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

func WatchLogDir(dir string) {
	type fileState struct {
		size    int64
		modTime time.Time
	}
	states := make(map[string]fileState)

	for {
		time.Sleep(1 * time.Second)

		var currentFiles []FileInfo

		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
					continue
				}
				info, err := e.Info()
				if err != nil {
					continue
				}
				currentFiles = append(currentFiles, FileInfo{
					Name: e.Name(),
					Size: info.Size(),
				})
			}
		}

		processDir := filepath.Join(dir, "process")
		if entries, err := os.ReadDir(processDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
					continue
				}
				info, err := e.Info()
				if err != nil {
					continue
				}
				currentFiles = append(currentFiles, FileInfo{
					Name: "process/" + e.Name(),
					Size: info.Size(),
				})
			}
		}

		if len(currentFiles) != len(states) {
			GlobalHub.Broadcast <- &WsMessage{Type: "files", Files: currentFiles}
		}

		for _, fi := range currentFiles {
			path := filepath.Join(dir, fi.Name)
			info, err := os.Stat(path)
			if err != nil {
				continue
			}

			prev, seen := states[fi.Name]
			if !seen || info.Size() != prev.size || info.ModTime() != prev.modTime {
				states[fi.Name] = fileState{size: info.Size(), modTime: info.ModTime()}
				if !seen {
					continue
				}

				GlobalHub.mu.RLock()
				hasSubscribers := len(GlobalHub.rooms[fi.Name]) > 0
				GlobalHub.mu.RUnlock()

				if !hasSubscribers {
					continue
				}

				lines, total, err := readLogTail(path, 300)
				if err != nil {
					log.Printf("⚠️ WatchLogDir read %s: %v", fi.Name, err)
					continue
				}

				GlobalHub.Broadcast <- &WsMessage{
					Type:  "log",
					Room:  fi.Name,
					Lines: lines,
					Total: total,
					Count: len(lines),
				}
			}
		}
	}
}
