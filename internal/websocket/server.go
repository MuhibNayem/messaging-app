package websocket

import (
	"log"
	"net/http"
	"time"

	"messaging-app/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const wsAuthProtocol = "connectify.auth"

// ServeWs handles new websocket connections and registers them with the Hub.
func ServeWs(c *gin.Context, hub *Hub) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			// TODO: restrict allowed origins
			return true
		},
		Subprotocols: []string{wsAuthProtocol},
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	userID, err := utils.GetUserIDFromContext(c)
	if err != nil || userID.IsZero() {
		log.Printf("Unauthorized WS attempt")
		conn.Close()
		return
	}

	groups, err := hub.groupRepo.GetUserGroups(c.Request.Context(), userID)
	if err != nil {
		log.Printf("Error fetching groups: %v", err)
	}

	listeners := make(map[string]bool)
	for _, g := range groups {
		listeners[g.ID.Hex()] = true
	}

	client := &Client{
		userID:    userID.Hex(),
		conn:      conn,
		send:      make(chan []byte, 256),
		lastSeen:  time.Now(),
		listeners: listeners,
	}

	hub.register <- client
	go client.writePump()
	go client.readPump(hub)
}
