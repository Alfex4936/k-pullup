package handler

import (
	"bytes"
	"context"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/service"
	"github.com/Alfex4936/chulbong-kr/util"
	sonic "github.com/bytedance/sonic"
	"github.com/rs/xid"

	"strings"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

type ChatHandler struct {
	ChatService *service.ChatService

	ChatUtil    *util.ChatUtil
	BadWordUtil *util.BadWordUtil
}

// NewChatHandler creates a new ChatHandler with dependencies injected
func NewChatHandler(chat *service.ChatService, cutil *util.ChatUtil, butil *util.BadWordUtil,
) *ChatHandler {
	return &ChatHandler{
		ChatService: chat,
		ChatUtil:    cutil,
		BadWordUtil: butil,
	}
}

// RegisterChatRoutes sets up the routes for chat handling within the application.
func RegisterChatRoutes(api fiber.Router, websocketConfig websocket.Config, handler *ChatHandler) {
	api.Get("/ws/:markerID", func(c *fiber.Ctx) error {
		// TODO: update on adding ban feature on admin API in frontend
		// Extract markerID from the parameter
		// markerID := c.Params("markerID")
		// reqID := c.Query("request-id")

		// Use GetBanDetails to check if the user is banned and get the remaining ban time
		// banned, remainingTime, err := handler.ChatService.GetBanDetails(markerID, reqID)
		// if err != nil {
		// 	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Internal server error"})
		// }
		// if banned {
		// 	return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
		// 		"error":         "User is banned",
		// 		"remainingTime": remainingTime.Seconds(), // Respond with remaining time in seconds
		// 	})
		// }

		// Proceed with WebSocket upgrade if not banned
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	}, websocket.New(func(c *websocket.Conn) {
		// Extract markerID from the parameter again if necessary
		markerID := c.Params("markerID")
		reqID := c.Query("request-id")

		// Now, the connection is already upgraded to WebSocket, and passed the ban check.
		handler.HandleChatRoom(c, markerID, reqID)
	}, websocketConfig))
}

// HandleChatRoomHandler manages WebSocket chat connections for markers.
//
// @Summary WebSocket chat connection
// @Description Establishes a WebSocket connection for a specific marker chat room.
// @ID ws-chat-room
// @Tags chats
// @Accept json
// @Produce json
// @Security
// @Param markerID path string true "Marker ID for the chat room"
// @Param request-id query string false "Unique client request ID"
// @Success 101 "Switching Protocols - WebSocket connection established"
// @Failure 400 {object} map[string]string "Invalid marker ID"
// @Failure 403 {object} map[string]interface{} "User is banned from the chat room"
// @Failure 426 {object} map[string]string "Upgrade required to WebSocket"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/ws/{markerID} [get]
func (h *ChatHandler) HandleChatRoom(c *websocket.Conn, markerID, reqID string) {
	// clientID := c.Locals("userID").(int)
	// clientNickname := c.Locals("username").(string)
	if markerID == "" || strings.Contains(markerID, "&") {
		c.WriteJSON(dto.SimpleErrorResponse{Error: "wrong marker id"})
		c.Close()
		return
	}
	clientID := reqID

	exists, _ := h.ChatService.CheckDuplicateConnectionByLocal(markerID, clientID)
	if exists {
		c.WriteJSON(dto.SimpleErrorResponse{Error: "duplicate connection"})
		c.Close()
		return
	}

	// clientID := rand.Int()

	// clientNickname := "user-" + uuid.New().String()
	clientNickname := h.ChatUtil.GenerateKoreanNickname()
	conn, saved, _ := h.ChatService.SaveConnection(markerID, clientID, clientNickname, c)
	if !saved || conn == nil {
		c.WriteJSON(dto.SimpleErrorResponse{Error: "Failed to save connection"})
		c.Close()
		return
	}

	// services.AddConnectionRoomToRedis(markerID, clientID, clientNickname) // saves to redis, "room:%s:connections"

	defer func() {
		// Get the nickname before removing the connection
		nickname, err := h.ChatService.GetNickname(markerID, clientID)

		// Remove the client from the room
		h.ChatService.RemoveWsFromRoom(markerID, clientID)

		if err == nil {
			// Broadcast leave message after removing the connection
			h.ChatService.BroadcastMessageToRoom(markerID, nickname+" 님이 퇴장하셨습니다.", nickname, clientID)
			// Broadcast updated user count
			h.ChatService.BroadcastUserCountToRoomByLocal(markerID)
		}
	}()

	// c.SetPingHandler(func(appData string) error {
	// 	// Respond with a pong
	// 	return c.WriteMessage(websocket.PongMessage, []byte(appData))
	// })

	// After saving the connection and broadcasting the join message
	// Retrieve recent messages
	ctx := context.Background()
	messages, err := h.ChatService.GetRecentMessages(ctx, markerID)
	if err != nil {
		// Handle error
	} else {
		// Send recent messages to the user
		for _, msg := range messages {
			payload, err := sonic.Marshal(msg)
			if err != nil {
				// Handle error
				continue
			}
			select {
			case conn.Send <- payload:
				// Message enqueued to be sent by writePump goroutine
			default:
				// Handle full send channel
			}
		}
	}

	// Broadcast join message
	// broadcasts directly by app memory objects
	// services.PublishMessageToAMQP(context.Background(), markerID, clientNickname+" 님이 입장하셨습니다.", clientNickname, clientID)
	h.ChatService.BroadcastMessageToRoom(markerID, clientNickname+" 님이 입장하셨습니다.", clientNickname, clientID)
	h.ChatService.BroadcastUserCountToRoomByLocal(markerID) // sends how many users in the room

	for {
		if err := c.SetReadDeadline(time.Now().Add(time.Second * 60)); err != nil {
			break
		}
		_, message, err := c.ReadMessage()
		if err != nil {
			// log.Printf("Error reading message: %v", err)
			break
		}

		if bytes.Equal(message, []byte(`{"type":"ping"}`)) {
			// if mType == 9 || mType == 10 {
			h.ChatService.UpdateLastPing(markerID, clientID)
			// if err := c.WriteMessage(websocket.TextMessage, []byte("pong")); err != nil {
			// 	log.Printf("Error sending 'pong': %v", err)
			// }
			continue // Skip further processing for this message
		}

		message = bytes.TrimSpace(message)
		if len(message) == 0 {
			continue
		}

		// Then, replace bad words with asterisks in the message string
		cleanMessage, err := h.BadWordUtil.ReplaceBadWordsInBytes(message)
		if len(cleanMessage) == 0 && err != nil {
			// log.Printf("Error replacing bad words: %v", err)
			continue
		}

		// log.Printf("❤️ received message: %v", cleanMessage)

		// Publish the valid message to the RabbitMQ queue for this chat room
		// services.PublishMessageToAMQP(context.Background(), markerID, cleanMessage, clientNickname, clientID)

		// Broadcast received message
		h.ChatService.UpdateLastPing(markerID, clientID)

		// Create the broadcast message
		broadcastMsg := dto.BroadcastMessage{
			UID:          xid.New().String(),
			Message:      util.BytesToString(cleanMessage),
			UserID:       clientID,
			UserNickname: clientNickname,
			RoomID:       markerID,
			Timestamp:    time.Now().UnixMilli(),
		}

		// Save the message to Redis
		if err := h.ChatService.SaveMessageToRedis(ctx, broadcastMsg); err != nil {
			// Handle error
		}

		// Broadcast the message
		if err := h.ChatService.BroadcastMessageToRoomByDTO(broadcastMsg); err != nil {
			break
		}
	}
}
