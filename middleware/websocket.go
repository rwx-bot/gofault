package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gofault/gofault/core"
	"github.com/gorilla/websocket"
)

// WebSocketConfig holds configuration for the WebSocket middleware.
type WebSocketConfig struct {
	// Enabled enables the WebSocket middleware.
	Enabled bool
	// ReadBufferSize is the buffer size for reading messages.
	ReadBufferSize int
	// WriteBufferSize is the buffer size for writing messages.
	WriteBufferSize int
	// ReadTimeout is the read deadline for the connection.
	ReadTimeout time.Duration
	// WriteTimeout is the write deadline for the connection.
	WriteTimeout time.Duration
	// PingInterval is the interval for sending ping frames.
	PingInterval time.Duration
	// CheckOrigin controls origin validation.
	CheckOrigin func(r *http.Request) bool
}

// DefaultWebSocketConfig returns a default WebSocket configuration.
func DefaultWebSocketConfig() WebSocketConfig {
	return WebSocketConfig{
		Enabled:         true,
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		ReadTimeout:     60 * time.Second,
		WriteTimeout:    60 * time.Second,
		PingInterval:    30 * time.Second,
		CheckOrigin:     nil, // default: same origin check
	}
}

// WebSocketHandler defines the interface for WebSocket message handlers.
type WebSocketHandler interface {
	// HandleMessage is called when a text or binary message is received.
	HandleMessage(ctx *core.Ctx, conn *websocket.Conn, messageType int, data []byte) error
	// HandleConnect is called when a new WebSocket connection is established.
	HandleConnect(ctx *core.Ctx, conn *websocket.Conn) error
	// HandleDisconnect is called when the WebSocket connection is closed.
	HandleDisconnect(ctx *core.Ctx, conn *websocket.Conn)
}

// WebSocketHandlerFunc is an adapter that allows a plain function to satisfy WebSocketHandler.
// Nil fields are treated as no-ops.
type WebSocketHandlerFunc struct {
	// OnMessage is called for each WebSocket message. Required.
	OnMessage func(ctx *core.Ctx, conn *websocket.Conn, messageType int, data []byte) error
	// OnConnect is called when the connection is established. Optional.
	OnConnect func(ctx *core.Ctx, conn *websocket.Conn) error
	// OnDisconnect is called when the connection is closed. Optional.
	OnDisconnect func(ctx *core.Ctx, conn *websocket.Conn)
}

// HandleMessage implements WebSocketHandler.
func (h WebSocketHandlerFunc) HandleMessage(ctx *core.Ctx, conn *websocket.Conn, messageType int, data []byte) error {
	if h.OnMessage == nil {
		return nil
	}
	return h.OnMessage(ctx, conn, messageType, data)
}

// HandleConnect implements WebSocketHandler.
func (h WebSocketHandlerFunc) HandleConnect(ctx *core.Ctx, conn *websocket.Conn) error {
	if h.OnConnect == nil {
		return nil
	}
	return h.OnConnect(ctx, conn)
}

// HandleDisconnect implements WebSocketHandler.
func (h WebSocketHandlerFunc) HandleDisconnect(ctx *core.Ctx, conn *websocket.Conn) {
	if h.OnDisconnect != nil {
		h.OnDisconnect(ctx, conn)
	}
}

// WebSocketMiddleware creates a middleware that upgrades HTTP to WebSocket.
func WebSocketMiddleware(config WebSocketConfig, handler WebSocketHandler) core.MiddlewareFunc {
	if !config.Enabled {
		return nil
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  config.ReadBufferSize,
		WriteBufferSize: config.WriteBufferSize,
		CheckOrigin:     config.CheckOrigin,
	}

	return func(ctx *core.Ctx, next core.Handler) error {
		conn, err := upgrader.Upgrade(ctx.Response, ctx.Request, nil)
		if err != nil {
			return err
		}
		defer conn.Close()

		// Set deadlines. A zero timeout means "no deadline" for the connection,
		// not "expire immediately".
		if config.ReadTimeout > 0 {
			conn.SetReadDeadline(time.Now().Add(config.ReadTimeout))
		}
		if config.WriteTimeout > 0 {
			conn.SetWriteDeadline(time.Now().Add(config.WriteTimeout))
		}

		// Start ping handler. A zero or negative interval disables pinging;
		// time.NewTicker panics on a non-positive duration.
		stopCh := make(chan struct{})
		var wg sync.WaitGroup
		if config.PingInterval > 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := time.NewTicker(config.PingInterval)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						writeDeadline := time.Time{}
						if config.WriteTimeout > 0 {
							writeDeadline = time.Now().Add(config.WriteTimeout)
						}
						if err := conn.WriteControl(websocket.PingMessage, nil, writeDeadline); err != nil {
							return
						}
					case <-stopCh:
						return
					}
				}
			}()
		}

		// Handle connect callback
		if err := handler.HandleConnect(ctx, conn); err != nil {
			close(stopCh)
			wg.Wait()
			return err
		}

		// Message read loop
		var readErr error
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				readErr = err
				break
			}
			if config.ReadTimeout > 0 {
				conn.SetReadDeadline(time.Now().Add(config.ReadTimeout))
			}
			if err := handler.HandleMessage(ctx, conn, msgType, data); err != nil {
				readErr = err
				break
			}
		}

		close(stopCh)
		wg.Wait()

		handler.HandleDisconnect(ctx, conn)

		// Don't treat normal WebSocket close as an error
		if websocket.IsUnexpectedCloseError(readErr, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
			return readErr
		}
		return nil
	}
}

// WebSocketHijackMiddleware creates a middleware that upgrades HTTP to WebSocket
// and stores the connection in ctx.Locals for use by the next handler.
// This enables WebSocket routes to be registered like regular HTTP routes.
func WebSocketHijackMiddleware(config WebSocketConfig) core.MiddlewareFunc {
	if !config.Enabled {
		return nil
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  config.ReadBufferSize,
		WriteBufferSize: config.WriteBufferSize,
		CheckOrigin:     config.CheckOrigin,
	}

	return func(ctx *core.Ctx, next core.Handler) error {
		conn, err := upgrader.Upgrade(ctx.Response, ctx.Request, nil)
		if err != nil {
			return err
		}

		ctx.SetLocal("ws_conn", conn)
		ctx.SetLocal("ws_upgrader", &upgrader)

		return next(ctx)
	}
}
