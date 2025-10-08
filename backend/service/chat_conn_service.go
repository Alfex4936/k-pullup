package service

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"time"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/rs/xid"
	"github.com/vmihailenco/msgpack/v5"

	sonic "github.com/bytedance/sonic"
	"github.com/gofiber/contrib/websocket"
	"github.com/redis/rueidis"
)

// LAVINMQ: Ensure only one subscription per room
// if _, exists := ActiveSubscriptions.Load(key); !exists {
// 	go SubscribeAndBroadcastFromAMQP(markerID)
// 	ActiveSubscriptions.Store(key, struct{}{}) // marker_%s
// }

const (
	MAX_MESSAGE_RETAIN = 10 * 24 // days*hours
)

// SaveConnection stores a WebSocket connection associated with a markerID in app memory
func (s *ChatService) SaveConnection(markerID, clientID, clientNickname string, conn *websocket.Conn) (*ChulbongConn, bool, error) {
	// s.Logger.Info("Saving connection", zap.String("markerID", markerID), zap.String("clientID", clientID))

	// Get or create the inner map for the room
	roomConns, _ := s.WebSocketManager.rooms.LoadOrCompute(markerID, func() *xsync.MapOf[string, *ChulbongConn] {
		return xsync.NewMapOf[string, *ChulbongConn]()
	})

	// Check if the clientID already exists in the inner map
	c, loaded := roomConns.LoadOrStore(clientID, func() *ChulbongConn {
		// Create the new connection object
		newConn := &ChulbongConn{
			Socket:       conn,
			UserID:       clientID,
			Nickname:     clientNickname,
			Send:         make(chan []byte, 256), // Buffered channel
			InActiveChan: make(chan struct{}),
		}
		newConn.UpdateLastSeen()

		// Start the writePump in a separate goroutine
		go newConn.writePump()

		return newConn
	}())

	if loaded {
		// Connection for clientID already exists, avoid adding a duplicate
		return nil, false, errors.New("duplicate connection")
	}

	// No duplicate found, connection has been added successfully
	return c, true, nil
}

// REDIS
// To see which users are in a room easily (in case distributed servers)
func (s *ChatService) AddConnectionRoomToRedis(markerID, userID, username string) error {
	ctx := context.Background()
	key := fmt.Sprintf("room:%s:connections", markerID)
	connID := xid.New().String() // unique identifier for the connection

	connInfo := dto.ConnectionInfo{
		UserID:   userID,
		RoomID:   markerID,
		Username: username,
		ConnID:   connID,
	}

	jsonConnInfo, err := sonic.Marshal(connInfo)
	if err != nil {
		return err
	}

	// Use HSET to set the field value
	setCmd := s.Redis.Core.Client.B().Hset().
		Key(key).
		FieldValue().
		FieldValue(userID, rueidis.BinaryString(jsonConnInfo)).
		Build()

	// Execute the HSET command
	if err := s.Redis.Core.Client.Do(ctx, setCmd).Error(); err != nil {
		return err
	}

	// Set expiration on the whole hash key
	expireCmd := s.Redis.Core.Client.B().Expire().Key(key).Seconds(int64(time.Hour / time.Second)).Build()
	if err := s.Redis.Core.Client.Do(ctx, expireCmd).Error(); err != nil {
		return err
	}
	return nil
}

func (s *ChatService) CheckDuplicateConnection(markerID, userID string) (bool, error) {
	ctx := context.Background()
	key := fmt.Sprintf("room:%s:connections", markerID)

	// Build the HEXISTS command
	cmd := s.Redis.Core.Client.B().Hexists().Key(key).Field(userID).Build()

	// Execute the HEXISTS command
	exists, err := s.Redis.Core.Client.Do(ctx, cmd).AsBool()
	if err != nil {
		return false, err // Proper error handling
	}

	return exists, nil
}

func (s *ChatService) CheckDuplicateConnectionByLocal(markerID, clientID string) (bool, error) {
	roomConns, ok := s.WebSocketManager.rooms.Load(markerID)
	if !ok {
		return false, nil // No room found, so no duplicate
	}

	_, exists := roomConns.Load(clientID)
	return exists, nil
}

func (s *ChatService) UpdateLastPing(markerID, clientID string) {
	if roomConns, ok := s.WebSocketManager.rooms.Load(markerID); ok {
		if conn, ok := roomConns.Load(clientID); ok {
			conn.UpdateLastSeen()
		}
	}
}

func (s *ChatService) GetRecentMessages(ctx context.Context, roomID string) ([]dto.BroadcastMessage, error) {
	key := fmt.Sprintf("chat:room:%s:messages", roomID)
	now := time.Now().UnixMilli()
	sevenDaysAgo := now - MAX_MESSAGE_RETAIN*60*60*1000 // 7 days in milliseconds

	min := strconv.FormatInt(sevenDaysAgo, 10)
	max := strconv.FormatInt(now, 10)

	// Build the ZRANGEBYSCORE command
	zrangeCmd := s.Redis.Core.Client.B().Zrangebyscore().Key(key).Min(min).Max(max).Build()

	// Execute the command and get the result as an array
	redisResult, err := s.Redis.Core.Client.Do(ctx, zrangeCmd).ToArray()
	if err != nil {
		return nil, err
	}

	// Deserialize messages
	messages := make([]dto.BroadcastMessage, len(redisResult))
	for i, msgRedisMessage := range redisResult {
		// Get the message data as []byte
		msgData, err := msgRedisMessage.AsBytes()
		if err != nil {
			return nil, err
		}

		var msg dto.BroadcastMessage
		// Decode the message
		err = msgpack.Unmarshal(msgData, &msg)
		if err != nil {
			return nil, err
		}

		messages[i] = msg
	}

	// Deserialize messages
	// messages := make([]dto.BroadcastMessage, len(result))
	// for i, msgData := range result {
	// 	compressedMsg := []byte(msgData)
	// 	decompressedMsg, err := decompress(compressedMsg)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	var msg dto.BroadcastMessage
	// 	if err := sonic.Unmarshal(decompressedMsg, &msg); err != nil {
	// 		return nil, err
	// 	}
	// 	messages[i] = msg
	// }

	return messages, nil
}

func (s *ChatService) CleanupOldMessages(ctx context.Context, roomID string) error {
	key := fmt.Sprintf("chat:room:%s:messages", roomID)
	sevenDaysAgo := time.Now().Add(-MAX_MESSAGE_RETAIN * time.Hour).UnixMilli()

	// Build the ZREMRANGEBYSCORE command
	zremCmd := s.Redis.Core.Client.B().Zremrangebyscore().Key(key).Min("0").Max(strconv.FormatInt(sevenDaysAgo, 10)).Build()

	// Execute the command
	return s.Redis.Core.Client.Do(ctx, zremCmd).Error()
}

func (s *ChatService) CleanupAllRooms(ctx context.Context) {
	s.WebSocketManager.rooms.Range(func(roomID string, _ *xsync.MapOf[string, *ChulbongConn]) bool {
		_ = s.CleanupOldMessages(ctx, roomID)
		return true
	})
}

func (s *ChatService) SaveMessageToRedis(ctx context.Context, message dto.BroadcastMessage) error {
	key := fmt.Sprintf("chat:room:%s:messages", message.RoomID)
	score := float64(message.Timestamp)

	// WAY: sonic json
	// Serialize the message using Sonic
	// msgJSON, err := sonic.ConfigFastest.Marshal(message)
	// if err != nil {
	// 	return err
	// }

	// WAY: msgpack
	// Create a buffer
	var buf bytes.Buffer

	// Create an encoder and set it to use array encoding
	enc := msgpack.NewEncoder(&buf)
	enc.UseArrayEncodedStructs(true)

	// Serialize the message using MessagePack
	if err := enc.Encode(message); err != nil {
		return err
	}
	msgPackData := buf.Bytes()

	// WAY: compress
	// Compress the message
	// compressedMsg, err := compress(msgJSON)
	// if err != nil {
	// 	return err
	// }

	// Build the ZADD command
	zaddCmd := s.Redis.Core.Client.B().Zadd().Key(key).ScoreMember().
		ScoreMember(score, rueidis.BinaryString(msgPackData)).Build()

	// Execute the command
	return s.Redis.Core.Client.Do(ctx, zaddCmd).Error()
}

// TODO: call this before adding a new message to limit the number of messages in a room
func (s *ChatService) EnforceMaxMessagesPerRoom(ctx context.Context, roomID string, maxMessages int64) error {
	key := fmt.Sprintf("chat:room:%s:messages", roomID)
	// Remove messages exceeding maxMessages
	return s.Redis.Core.Client.Do(ctx,
		s.Redis.Core.Client.B().Zremrangebyrank().Key(key).Start(0).Stop(-maxMessages-1).Build(),
	).Error()
}

func (s *RoomConnectionManager) StartConnectionChecker() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.CheckConnections()
	}
}

// CheckConnections iterates over all connections and sends a ping message.
// Connections that fail to respond can be considered inactive and removed.
func (s *RoomConnectionManager) CheckConnections() {
	gracePeriod := 10 * time.Minute
	now := time.Now()

	s.rooms.Range(func(markerID string, roomConns *xsync.MapOf[string, *ChulbongConn]) bool {
		roomConns.Range(func(clientID string, conn *ChulbongConn) bool {
			if now.Sub(conn.GetLastSeen()) > gracePeriod {
				select {
				case conn.InActiveChan <- struct{}{}:
					// Inactive signal sent
				default:
					// Channel is full; handle accordingly
				}
				roomConns.Delete(clientID)
			}
			return true
		})
		if roomConns.Size() == 0 {
			s.rooms.Delete(markerID)
		}
		return true
	})
}

func (s *ChatService) RemoveConnectionFromRedis(markerID, xRequestID string) {
	ctx := context.Background()
	key := fmt.Sprintf("room:%s:connections", markerID)

	// Use HDEL to remove the connection information, indexed by the UserID
	cmd := s.Redis.Core.Client.B().Hdel().Key(key).Field(xRequestID).Build()
	if err := s.Redis.Core.Client.Do(ctx, cmd).Error(); err != nil {
		enqueueRemovalTask(markerID, xRequestID)
	}
}

// RemoveConnection removes a WebSocket connection associated with a id
func (s *ChatService) RemoveWsFromRoom(markerID, clientID string) (string, error) {
	if roomConns, ok := s.WebSocketManager.rooms.Load(markerID); ok {
		if conn, ok := roomConns.Load(clientID); ok {
			clientNickname := conn.Nickname
			close(conn.Send)
			close(conn.InActiveChan)
			conn.Socket.Close()
			roomConns.Delete(clientID)

			// s.Logger.Info("Connection closed", zap.String("markerID", markerID), zap.String("clientID", clientID))

			if roomConns.Size() == 0 {
				s.WebSocketManager.rooms.Delete(markerID)
			}
			return clientNickname, nil
		}
	}
	return "", errors.New("connection not found")
}

// KickUserFromRoom closes the connection for a user in a specified room.
func (s *ChatService) KickUserFromRoom(markerID, clientID string) error {
	if roomConns, ok := s.WebSocketManager.rooms.Load(markerID); ok {
		if conn, ok := roomConns.Load(clientID); ok {
			close(conn.Send)
			close(conn.InActiveChan)
			conn.Socket.Close()
			roomConns.Delete(clientID)

			if roomConns.Size() == 0 {
				s.WebSocketManager.rooms.Delete(markerID)
			}
			return nil
		}
		return errors.New("user not found in room")
	}
	return errors.New("room not found")
}

func (s *ChatService) GetUserCountInRoom(ctx context.Context, markerID string) (int64, error) {
	key := fmt.Sprintf("room:%s:connections", markerID)

	// Use HLEN to get the number of connections in the room
	cmd := s.Redis.Core.Client.B().Hlen().Key(key).Build()
	return s.Redis.Core.Client.Do(ctx, cmd).AsInt64()
}

func (s *ChatService) GetUserCountInRoomByLocal(markerID string) (int, error) {
	if roomConns, ok := s.WebSocketManager.rooms.Load(markerID); ok {
		return int(roomConns.Size()), nil
	}
	return 0, nil
}

// GetActiveUsersInRoom returns a list of active users in a room
func (s *ChatService) GetActiveUsersInRoom(markerID string) ([]dto.ConnectionInfo, error) {
	roomConns, ok := s.WebSocketManager.rooms.Load(markerID)
	if !ok {
		return nil, errors.New("room not found")
	}

	var users []dto.ConnectionInfo
	roomConns.Range(func(clientID string, conn *ChulbongConn) bool {
		users = append(users, dto.ConnectionInfo{
			UserID:   conn.UserID,
			RoomID:   markerID,
			Username: conn.Nickname,
			ConnID:   clientID,
		})
		return true
	})

	return users, nil
}

// GetAllRedisConnectionsFromRoom retrieves all connection information for a given room.
func (s *ChatService) GetAllRedisConnectionsFromRoom(markerID string) ([]dto.ConnectionInfo, error) {
	ctx := context.Background()
	key := fmt.Sprintf("room:%s:connections", markerID)

	// Use HGETALL to retrieve all field-value pairs from the hash
	cmd := s.Redis.Core.Client.B().Hgetall().Key(key).Build()

	// Execute the command and get the result as a string map
	result, err := s.Redis.Core.Client.Do(ctx, cmd).AsStrMap()
	if err != nil {
		return nil, err
	}

	// Deserialize the connection information from JSON stored in each field
	connections := make([]dto.ConnectionInfo, 0, len(result))
	for _, jsonConnInfo := range result {
		var connInfo dto.ConnectionInfo
		// Use StringToBytes to avoid unnecessary memory allocation
		jsonBytes := util.StringToBytes(jsonConnInfo)

		if err := sonic.ConfigFastest.Unmarshal(jsonBytes, &connInfo); err != nil {
			log.Printf("Error unmarshaling connection info: %v", err)
			continue
		}
		connections = append(connections, connInfo)
	}

	return connections, nil
}

func (s *ChatService) BanUser(markerID, userID string, duration time.Duration) error {
	// First, send a ban notification message to the user if they're connected
	if roomConns, ok := s.WebSocketManager.rooms.Load(markerID); ok {
		if conn, ok := roomConns.Load(userID); ok {
			// Send ban notification to the specific user
			banMessage := fmt.Sprintf("⚠️ You have been banned for %.0f minutes. Reason: Administrative action.", duration.Minutes())
			banPayload := createMessagePayload(markerID, banMessage, "System", "system")

			select {
			case conn.Send <- banPayload:
				// Message sent successfully
			default:
				// Connection might be full or closed
			}

			// Give a moment for the message to be sent
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Then, kick the user from the room.
	err := s.KickUserFromRoom(markerID, userID)
	if err != nil {
		log.Printf("Error kicking user %s from room %s: %v", userID, markerID, err)
	}

	// Finally, set the ban in Redis.
	banKey := fmt.Sprintf("ban_%s_%s", markerID, userID)
	ctx := context.Background()

	// Use Set to set the ban in Redis
	cmd := s.Redis.Core.Client.B().Set().Key(banKey).Value("banned").Nx().Ex(duration).Build()
	if err := s.Redis.Core.Client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("failed to set ban in Redis: %w", err)
	}

	// Broadcast to room that user was banned (optional)
	nickname, _ := s.GetNickname(markerID, userID)
	if nickname == "" {
		nickname = "사용자"
	}

	banAnnouncementMsg := fmt.Sprintf("🚫 %s님이 관리자에 의해 차단되었습니다.", nickname)
	s.BroadcastMessageToRoom(markerID, banAnnouncementMsg, "System", "system")

	return nil
}

func (s *ChatService) GetBanDetails(markerID, userID string) (bool, time.Duration, error) {
	banKey := "ban_" + markerID + "_" + userID
	ctx := context.Background()

	// Use PTTL to get the remaining TTL in milliseconds
	cmd := s.Redis.Core.Client.B().Pttl().Key(banKey).Build()
	ttl, err := s.Redis.Core.Client.Do(ctx, cmd).AsInt64()
	if err != nil {
		return false, 0, err
	}

	if ttl == -2 {
		// Key does not exist
		return false, 0, nil
	}

	banned := true
	var remainingTime time.Duration
	if ttl == -1 {
		// Key exists but no expiration is set (banned indefinitely)
		remainingTime = 0
	} else {
		remainingTime = time.Duration(ttl) * time.Millisecond
	}

	return banned, remainingTime, nil
}

func (conn *ChulbongConn) writePump() {
	for {
		select {
		case <-conn.InActiveChan:
			// conn.Socket.WriteJSON(fiber.Map{"error": "inactive connection"})
			// time.Sleep(500 * time.Millisecond)
			conn.Socket.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "inactive"))
			// conn.Socket.Close()
			return
		case message, ok := <-conn.Send:
			if !ok {
				conn.Socket.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			// conn.Socket.SetWriteDeadline(time.Now().Add(30 * time.Second))

			if err := conn.Socket.WriteMessage(websocket.TextMessage, message); err != nil {
				// log.Printf("📆 Error sending text message: %v", err)
				continue
			}
		}
	}
}

func compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompress(data []byte) ([]byte, error) {
	buf := bytes.NewBuffer(data)
	zr, err := zlib.NewReader(buf)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	decompressedData, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	return decompressedData, nil
}
