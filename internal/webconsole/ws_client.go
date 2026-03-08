package webconsole

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type wsClient struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func newWSClient(conn *websocket.Conn) *wsClient {
	return &wsClient{conn: conn}
}

func (c *wsClient) send(event Event) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteJSON(event)
}

func (c *wsClient) ping() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second))
}

func (c *wsClient) closeAll() {}
