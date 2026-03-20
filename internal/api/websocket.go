package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *Server) websocketStream(c *gin.Context) {
	apiKey := c.Query("api_key")
	if !s.verifyAPIKey(apiKey) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err == nil {
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4401, "unauthorized"), time.Now().Add(time.Second))
			_ = conn.Close()
			return
		}
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid API key"})
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	queue := s.manager.Subscribe()
	defer s.manager.Unsubscribe(queue)
	conn.SetReadLimit(1024)
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()
	for {
		select {
		case msg, ok := <-queue:
			if !ok {
				return
			}
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		case <-pingTicker.C:
			if err := conn.WriteJSON(gin.H{"type": "ping"}); err != nil {
				return
			}
		}
	}
}
