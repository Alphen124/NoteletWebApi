package controllers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cloudinarysvc "noteletwebservice-development/services/cloudinary"

	"noteletwebservice-development/middlewares"
	"noteletwebservice-development/services/jwt"

	"github.com/gorilla/websocket"
)

// ─────────────────────────────────────────────
// Message types
// ─────────────────────────────────────────────
const (
	MsgTypeChat         = "chat"
	MsgTypeJoin         = "join"
	MsgTypeLeave        = "leave"
	MsgTypeTyping       = "typing"
	MsgTypeHistory      = "history"
	MsgTypeOnline       = "online"
	MsgTypeError        = "error"
	MsgTypeNotification = "notification"
)

// ─────────────────────────────────────────────
// Wire types
// ─────────────────────────────────────────────

// WireMessage is the JSON envelope exchanged over WebSocket
type WireMessage struct {
	Type       string        `json:"type"`
	RoomID     string        `json:"roomId"`
	Content    string        `json:"content,omitempty"`
	SenderID   int           `json:"senderId,omitempty"`
	SenderName string        `json:"senderName,omitempty"`
	Timestamp  time.Time     `json:"timestamp,omitempty"`
	MessageID  int           `json:"messageId,omitempty"`
	IsTyping   bool          `json:"isTyping,omitempty"`
	Online     []OnlineUser  `json:"online,omitempty"`
	History    []WireMessage `json:"history,omitempty"`
	ImageUrl   string        `json:"imageUrl,omitempty"`
	// Notification-specific fields
	DeviceID   int    `json:"deviceId,omitempty"`
	DeviceName string `json:"deviceName,omitempty"`
	NotifID    int    `json:"notifId,omitempty"`
}

// OnlineUser is a lightweight user presence descriptor
type OnlineUser struct {
	UserID int    `json:"userId"`
	Name   string `json:"name"`
}

// ─────────────────────────────────────────────
// Client
// ─────────────────────────────────────────────

// Client represents one connected WebSocket session
type Client struct {
	hub        *Hub
	conn       *websocket.Conn
	send       chan []byte
	userID     int
	name       string
	activeRoom string // room the client is currently in
	mu         sync.Mutex
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512 * 1024)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("chat: read error user=%d: %v", c.userID, err)
			}
			break
		}
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		var msg WireMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("chat: bad JSON from user=%d: %v", c.userID, err)
			continue
		}

		switch msg.Type {
		case MsgTypeChat:
			if (msg.Content == "" && msg.ImageUrl == "") || msg.RoomID == "" {
				continue
			}
			// Persist to DB and broadcast
			c.hub.handleChatMessage(c, msg)
		case MsgTypeTyping:
			c.hub.handleTyping(c, msg)
		case MsgTypeHistory:
			c.hub.sendHistory(c, msg.RoomID)
		}
	}
}

// ─────────────────────────────────────────────
// Hub
// ─────────────────────────────────────────────

// Hub maintains the set of active clients and routes messages
type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	db         *sql.DB
}

func newHub(db *sql.DB) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client, 16),
		unregister: make(chan *Client, 16),
		db:         db,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = true
			h.mu.Unlock()
			h.broadcastOnline()
			h.sendHistory(c, c.activeRoom)
			h.sendJoinNotice(c)

		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()
			h.broadcastOnline()
			h.sendLeaveNotice(c)
		}
	}
}

// broadcastOnline pushes the online user list to every room's members separately
func (h *Hub) broadcastOnline() {
	h.mu.RLock()
	roomClients := make(map[string][]*Client)
	for c := range h.clients {
		roomClients[c.activeRoom] = append(roomClients[c.activeRoom], c)
	}
	h.mu.RUnlock()

	for room, clients := range roomClients {
		list := make([]OnlineUser, 0, len(clients))
		for _, c := range clients {
			list = append(list, OnlineUser{UserID: c.userID, Name: c.name})
		}
		msg := WireMessage{Type: MsgTypeOnline, RoomID: room, Online: list, Timestamp: time.Now()}
		raw, _ := json.Marshal(msg)
		for _, c := range clients {
			select {
			case c.send <- raw:
			default:
			}
		}
	}
}

func (h *Hub) broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- data:
		default:
			// slow client — skip
		}
	}
}

// broadcastToRoom sends data only to clients in the specified room
func (h *Hub) broadcastToRoom(data []byte, roomName string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.activeRoom == roomName {
			select {
			case c.send <- data:
			default:
			}
		}
	}
}

func (h *Hub) sendToClient(c *Client, msg WireMessage) {
	raw, _ := json.Marshal(msg)
	select {
	case c.send <- raw:
	default:
	}
}

// handleChatMessage persists a chat message, broadcasts it, and notifies the device owner
func (h *Hub) handleChatMessage(c *Client, msg WireMessage) {
	msg.SenderID = c.userID
	msg.SenderName = c.name
	msg.Timestamp = time.Now()

	// Persist
	var msgID int
	var roomID int
	var imgURL interface{}
	if msg.ImageUrl != "" {
		imgURL = msg.ImageUrl
	}
	err := h.db.QueryRow(
		`INSERT INTO ChatMessage (RoomId, SenderId, Content, ImageUrl, CreatedAt)
		 VALUES ((SELECT RoomId FROM ChatRoom WHERE RoomName=$1 LIMIT 1), $2, $3, $4, $5)
		 RETURNING MessageId,
		           (SELECT RoomId FROM ChatRoom WHERE RoomName=$1 LIMIT 1)`,
		msg.RoomID, c.userID, msg.Content, imgURL, msg.Timestamp,
	).Scan(&msgID, &roomID)
	if err != nil {
		log.Printf("chat: persist error: %v", err)
	} else {
		msg.MessageID = msgID
	}

	raw, _ := json.Marshal(msg)
	h.broadcastToRoom(raw, msg.RoomID)

	// Notify the device owner if this is a device room ("dev-{id}")
	h.notifyDeviceOwner(c, msg, roomID)
	// Notify the renter when the device owner replies
	h.notifyRenter(c, msg, roomID)
}

// notifyDeviceOwner creates a DB notification and pushes a realtime event to the owner
func (h *Hub) notifyDeviceOwner(sender *Client, msg WireMessage, roomID int) {
	if roomID == 0 {
		return
	}
	// Find device and owner for this room
	var ownerID, deviceID int
	var deviceName string
	err := h.db.QueryRow(
		`SELECT d.DeviceNo, d.DeviceName, d.UserId
		 FROM ChatRoom r
		 JOIN Device d ON d.DeviceNo = r.DeviceId
		 WHERE r.RoomId = $1 AND r.DeviceId IS NOT NULL`,
		roomID,
	).Scan(&deviceID, &deviceName, &ownerID)
	if err != nil {
		// Not a device room — skip
		return
	}
	// Don't notify the owner if they are the one sending the message
	if ownerID == sender.userID {
		return
	}

	preview := msg.Content
	if len(preview) > 80 {
		preview = preview[:80] + "…"
	}

	// Persist notification
	var notifID int
	h.db.QueryRow(
		`INSERT INTO ChatNotification (OwnerId, RoomId, DeviceId, DeviceName, SenderName, Preview, IsRead, CreatedAt)
		 VALUES ($1, $2, $3, $4, $5, $6, false, $7)
		 RETURNING NotifId`,
		ownerID, roomID, deviceID, deviceName, sender.name, preview, time.Now(),
	).Scan(&notifID)

	// Push realtime notification if owner is currently connected
	notifMsg := WireMessage{
		Type:       MsgTypeNotification,
		RoomID:     msg.RoomID,
		DeviceID:   deviceID,
		DeviceName: deviceName,
		SenderName: sender.name,
		Content:    preview,
		NotifID:    notifID,
		Timestamp:  time.Now(),
	}
	notifRaw, _ := json.Marshal(notifMsg)

	h.mu.RLock()
	defer h.mu.RUnlock()
	for cl := range h.clients {
		if cl.userID == ownerID {
			select {
			case cl.send <- notifRaw:
			default:
			}
		}
	}
}

// notifyRenter creates a DB notification and pushes a realtime event to the renter
// when the device owner sends a message in the device-specific chat room.
func (h *Hub) notifyRenter(sender *Client, msg WireMessage, roomID int) {
	if roomID == 0 {
		return
	}
	// Room name formats:
	//   dev-{deviceId}-u-{renterId}
	//   dev-{deviceId}-u-{renterId}-req-{requestNo}
	var parsedDeviceID, renterID int
	n, err := fmt.Sscanf(msg.RoomID, "dev-%d-u-%d", &parsedDeviceID, &renterID)
	if err != nil || n != 2 || renterID == 0 {
		return // not a per-renter room
	}
	if renterID == sender.userID {
		return // sender is the renter — notifyDeviceOwner handles the other direction
	}

	// Confirm sender is the device owner
	var ownerID, deviceID int
	var deviceName string
	err = h.db.QueryRow(
		`SELECT d.DeviceNo, d.DeviceName, d.UserId
		 FROM ChatRoom r
		 JOIN Device d ON d.DeviceNo = r.DeviceId
		 WHERE r.RoomId = $1 AND r.DeviceId IS NOT NULL`,
		roomID,
	).Scan(&deviceID, &deviceName, &ownerID)
	if err != nil {
		return
	}
	if ownerID != sender.userID {
		return // sender is not the owner
	}

	preview := msg.Content
	if len(preview) > 80 {
		preview = preview[:80] + "…"
	}

	// Persist notification — OwnerId column is used as the recipient
	var notifID int
	h.db.QueryRow(
		`INSERT INTO ChatNotification (OwnerId, RoomId, DeviceId, DeviceName, SenderName, Preview, IsRead, CreatedAt)
		 VALUES ($1, $2, $3, $4, $5, $6, false, $7)
		 RETURNING NotifId`,
		renterID, roomID, deviceID, deviceName, sender.name, preview, time.Now(),
	).Scan(&notifID)

	// Push realtime notification to renter if connected
	notifMsg := WireMessage{
		Type:       MsgTypeNotification,
		RoomID:     msg.RoomID,
		DeviceID:   deviceID,
		DeviceName: deviceName,
		SenderName: sender.name,
		Content:    preview,
		NotifID:    notifID,
		Timestamp:  time.Now(),
	}
	notifRaw, _ := json.Marshal(notifMsg)

	h.mu.RLock()
	defer h.mu.RUnlock()
	for cl := range h.clients {
		if cl.userID == renterID {
			select {
			case cl.send <- notifRaw:
			default:
			}
		}
	}
}

// handleTyping broadcasts a typing indicator
func (h *Hub) handleTyping(c *Client, msg WireMessage) {
	out := WireMessage{
		Type:       MsgTypeTyping,
		RoomID:     msg.RoomID,
		SenderID:   c.userID,
		SenderName: c.name,
		IsTyping:   msg.IsTyping,
		Timestamp:  time.Now(),
	}
	raw, _ := json.Marshal(out)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for cl := range h.clients {
		if cl != c && cl.activeRoom == msg.RoomID {
			select {
			case cl.send <- raw:
			default:
			}
		}
	}
}

// sendHistory sends the last 50 messages for a room to a single client
func (h *Hub) sendHistory(c *Client, roomName string) {
	rows, err := h.db.Query(
		`SELECT m.MessageId, r.RoomName, o.FName || ' ' || o.LName AS SenderName,
		        m.SenderId, m.Content, COALESCE(m.ImageUrl,'') AS ImageUrl, m.CreatedAt
		 FROM ChatMessage m
		 JOIN ChatRoom r ON r.RoomId  = m.RoomId
		 JOIN AppUser  a ON a.UserId  = m.SenderId
		 JOIN Owner    o ON o.UserId  = a.UserId
		 WHERE r.RoomName = $1
		 ORDER BY m.CreatedAt DESC LIMIT 50`,
		roomName,
	)
	if err != nil {
		log.Printf("chat: history query error: %v", err)
		return
	}
	defer rows.Close()

	var history []WireMessage
	for rows.Next() {
		var m WireMessage
		m.Type = MsgTypeChat
		if err := rows.Scan(&m.MessageID, &m.RoomID, &m.SenderName, &m.SenderID, &m.Content, &m.ImageUrl, &m.Timestamp); err != nil {
			continue
		}
		history = append(history, m)
	}
	// Reverse so oldest first
	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}

	env := WireMessage{Type: MsgTypeHistory, RoomID: roomName, History: history, Timestamp: time.Now()}
	h.sendToClient(c, env)
}

func (h *Hub) sendJoinNotice(c *Client) {
	msg := WireMessage{
		Type:       MsgTypeJoin,
		RoomID:     c.activeRoom,
		SenderName: c.name,
		SenderID:   c.userID,
		Timestamp:  time.Now(),
	}
	raw, _ := json.Marshal(msg)
	h.broadcastToRoom(raw, c.activeRoom)
}

func (h *Hub) sendLeaveNotice(c *Client) {
	msg := WireMessage{
		Type:       MsgTypeLeave,
		RoomID:     c.activeRoom,
		SenderName: c.name,
		SenderID:   c.userID,
		Timestamp:  time.Now(),
	}
	raw, _ := json.Marshal(msg)
	h.broadcastToRoom(raw, c.activeRoom)
}

// ─────────────────────────────────────────────
// ChatController (HTTP handlers)
// ─────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins (CORS is handled by middleware)
		return true
	},
}

// ChatController exposes the WebSocket endpoint and REST helpers
type ChatController struct {
	hub *Hub
	db  *sql.DB
}

// NewChatController creates the controller and starts the hub goroutine
func NewChatController(db *sql.DB) *ChatController {
	hub := newHub(db)
	go hub.Run()
	return &ChatController{hub: hub, db: db}
}

// ServeWS handles WebSocket upgrade at GET /api/chat/ws?token=<jwt>
func (cc *ChatController) ServeWS(w http.ResponseWriter, r *http.Request) {
	// --- Authenticate via query param (WS cannot send headers easily) ---
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, "token required", http.StatusUnauthorized)
		return
	}
	claims, err := jwt.ValidateAccessToken(tokenStr)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// --- Fetch user display name ---
	var fname, lname string
	err = cc.db.QueryRow(
		`SELECT o.FName, o.LName FROM Owner o WHERE o.UserId = $1 LIMIT 1`,
		claims.UserId,
	).Scan(&fname, &lname)
	if err != nil {
		// Fall back to email prefix
		fname = claims.Email
		lname = ""
	}
	displayName := fname
	if lname != "" {
		displayName = fname + " " + lname
	}

	// --- Resolve room (default: general) ---
	roomName := r.URL.Query().Get("room")
	if roomName == "" {
		roomName = "general"
	}
	var roomExists int
	if dbErr := cc.db.QueryRow(`SELECT COUNT(*) FROM ChatRoom WHERE RoomName=$1`, roomName).Scan(&roomExists); dbErr != nil || roomExists == 0 {
		http.Error(w, "room not found", http.StatusBadRequest)
		return
	}

	// --- Upgrade ---
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("chat: upgrade error: %v", err)
		return
	}

	client := &Client{
		hub:        cc.hub,
		conn:       conn,
		send:       make(chan []byte, 256),
		userID:     claims.UserId,
		name:       displayName,
		activeRoom: roomName,
	}

	cc.hub.register <- client
	go client.writePump()
	go client.readPump()
}

// GetRooms returns available chat rooms (GET /api/chat/rooms)
func (cc *ChatController) GetRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rows, err := cc.db.Query(
		`SELECT RoomId, RoomName, CreatedAt FROM ChatRoom WHERE IsPublic = true ORDER BY RoomName`,
	)
	if err != nil {
		log.Printf("GetRooms: db error: %v", err)
		http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Room struct {
		ID        int       `json:"id"`
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"createdAt"`
	}
	var rooms []Room
	for rows.Next() {
		var rm Room
		if err := rows.Scan(&rm.ID, &rm.Name, &rm.CreatedAt); err == nil {
			rooms = append(rooms, rm)
		}
	}
	if rooms == nil {
		rooms = []Room{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rooms)
}

// EnsureDeviceRoom creates (if needed) a private per-renter room for a device negotiation.
// POST /api/chat/device-room  body: { "deviceId": 123, "renterId": 456 }
// renterId is optional. When the device owner calls this to join a renter's room,
// they supply the renter's userId so the room name matches what the renter created.
func (cc *ChatController) EnsureDeviceRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		DeviceID  int `json:"deviceId"`
		RenterID  int `json:"renterId"`  // optional: provided by owner to join renter's room
		RequestNo int `json:"requestNo"` // optional: scopes room to a specific rental request
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DeviceID <= 0 {
		http.Error(w, "invalid deviceId", http.StatusBadRequest)
		return
	}
	// Use provided renterId (owner joining renter's room), else use caller's own id (renter creating their room)
	roomUserId := userCtx.UserId
	if body.RenterID > 0 {
		roomUserId = body.RenterID
	}
	// Per-rental-request room when requestNo is supplied, otherwise fall back to per-renter room
	var roomName string
	if body.RequestNo > 0 {
		roomName = fmt.Sprintf("dev-%d-u-%d-req-%d", body.DeviceID, roomUserId, body.RequestNo)
	} else {
		baseRoom := fmt.Sprintf("dev-%d-u-%d", body.DeviceID, roomUserId)
		// If the base room already has a confirmed rental, generate a new sequenced room
		// so the renter starts a fresh chat for each new rental.
		roomName = baseRoom
		var confirmedCount int
		cc.db.QueryRow(
			`SELECT COUNT(*) FROM ChatMessage cm
			 JOIN ChatRoom cr ON cr.RoomId = cm.RoomId
			 WHERE cr.RoomName = $1 AND cm.Content = '__CONFIRMED__'`,
			baseRoom,
		).Scan(&confirmedCount)
		if confirmedCount > 0 {
			// Count existing rooms with this base to pick the next version number
			var existingCount int
			cc.db.QueryRow(
				`SELECT COUNT(*) FROM ChatRoom WHERE RoomName LIKE $1`,
				baseRoom+"%",
			).Scan(&existingCount)
			roomName = fmt.Sprintf("%s-%d", baseRoom, existingCount+1)
		}
	}
	if _, err := cc.db.Exec(
		`INSERT INTO ChatRoom (RoomName, IsPublic, DeviceId)
		 VALUES ($1, false, $2)
		 ON CONFLICT (RoomName) DO UPDATE SET DeviceId = EXCLUDED.DeviceId`,
		roomName, body.DeviceID,
	); err != nil {
		log.Printf("EnsureDeviceRoom: db error inserting room=%s deviceId=%d: %v", roomName, body.DeviceID, err)
		http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"roomName": roomName})
}

// UploadChatImage handles image uploads for chat messages.
// POST /api/chat/upload-image  multipart form field: image
func (cc *ChatController) UploadChatImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "file too large (max 10 MB)", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "image field required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Detect content type from first 512 bytes
	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	contentType := http.DetectContentType(buf[:n])
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(w, "only image files are allowed", http.StatusBadRequest)
		return
	}

	// Combine buffered bytes with the rest of the file
	combined := io.MultiReader(strings.NewReader(string(buf[:n])), file)

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		ext = ".jpg"
	}
	publicID := fmt.Sprintf("chat_%d", time.Now().UnixNano())
	imageURL, err := cloudinarysvc.UploadImage(r.Context(), combined, publicID, "notelet/chat")
	if err != nil {
		http.Error(w, "could not upload image: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": imageURL})
}

// ─────────────────────────────────────────────
// Notification endpoints
// ─────────────────────────────────────────────

// NotificationItem is a single notification row returned to the client
type NotificationItem struct {
	NotifID    int       `json:"notifId"`
	RoomName   string    `json:"roomName"`
	DeviceID   int       `json:"deviceId"`
	DeviceName string    `json:"deviceName"`
	SenderName string    `json:"senderName"`
	Preview    string    `json:"preview"`
	IsRead     bool      `json:"isRead"`
	CreatedAt  time.Time `json:"createdAt"`
}

// GetNotifications returns unread (and recent read) notifications for the authenticated user.
// GET /api/chat/notifications
func (cc *ChatController) GetNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rows, err := cc.db.Query(
		`SELECT n.NotifId, COALESCE(cr.RoomName,'') AS RoomName,
		        COALESCE(n.DeviceId, 0), COALESCE(n.DeviceName,''),
		        COALESCE(n.SenderName,''), COALESCE(n.Preview,''),
		        n.IsRead, n.CreatedAt
		 FROM ChatNotification n
		 LEFT JOIN ChatRoom cr ON cr.RoomId = n.RoomId
		 WHERE n.OwnerId = $1
		 ORDER BY n.IsRead ASC, n.CreatedAt DESC
		 LIMIT 40`,
		userCtx.UserId,
	)
	if err != nil {
		log.Printf("GetNotifications: db error userId=%d: %v", userCtx.UserId, err)
		http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []NotificationItem
	for rows.Next() {
		var it NotificationItem
		if err := rows.Scan(&it.NotifID, &it.RoomName, &it.DeviceID, &it.DeviceName,
			&it.SenderName, &it.Preview, &it.IsRead, &it.CreatedAt); err == nil {
			items = append(items, it)
		}
	}
	if items == nil {
		items = []NotificationItem{}
	}

	// Count unread
	var unread int
	for _, it := range items {
		if !it.IsRead {
			unread++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"notifications": items,
		"unread":        unread,
	})
}

// GetUnreadCount returns just the badge count for the nav bar.
// GET /api/chat/notifications/unread
func (cc *ChatController) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var count int
	cc.db.QueryRow(
		`SELECT COUNT(*) FROM ChatNotification WHERE OwnerId=$1 AND IsRead=false`,
		userCtx.UserId,
	).Scan(&count)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "unread": count})
}

// GetOwnerRooms returns all device chat rooms for devices owned by the authenticated user,
// with last message preview and unread notification count.
// GET /api/chat/owner-rooms
func (cc *ChatController) GetOwnerRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := cc.db.Query(
		`SELECT
		    cr.RoomName,
		    cr.DeviceId,
		    d.DeviceName,
		    COALESCE(d.ImageUrl, '') AS ImageUrl,
		    COALESCE(d.RentPrice, 0) AS RentPrice,
		    COALESCE(
		        (SELECT m.Content FROM ChatMessage m WHERE m.RoomId = cr.RoomId ORDER BY m.CreatedAt DESC LIMIT 1),
		        ''
		    ) AS LastMessage,
		    COALESCE(
		        (SELECT m.CreatedAt FROM ChatMessage m WHERE m.RoomId = cr.RoomId ORDER BY m.CreatedAt DESC LIMIT 1),
		        cr.CreatedAt
		    ) AS LastMessageAt,
		    COALESCE(
		        (SELECT o2.FName || ' ' || o2.LName
		         FROM ChatMessage m2
		         JOIN AppUser au2 ON au2.UserId = m2.SenderId
		         JOIN Owner o2 ON o2.UserId = au2.UserId
		         WHERE m2.RoomId = cr.RoomId
		         ORDER BY m2.CreatedAt DESC LIMIT 1),
		        ''
		    ) AS LastSenderName,
		    (SELECT COUNT(*) FROM ChatNotification n
		     WHERE n.RoomId = cr.RoomId AND n.OwnerId = $1 AND n.IsRead = false
		    ) AS UnreadCount
		FROM ChatRoom cr
		JOIN Device d ON d.DeviceNo = cr.DeviceId
		WHERE d.UserId = $1 AND cr.IsPublic = false
		ORDER BY LastMessageAt DESC`,
		userCtx.UserId,
	)
	if err != nil {
		log.Printf("GetOwnerRooms: db error userId=%d: %v", userCtx.UserId, err)
		http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type OwnerRoom struct {
		RoomName       string    `json:"roomName"`
		DeviceID       int       `json:"deviceId"`
		DeviceName     string    `json:"deviceName"`
		ImageUrl       string    `json:"imageUrl"`
		RentPrice      float64   `json:"rentPrice"`
		LastMessage    string    `json:"lastMessage"`
		LastMessageAt  time.Time `json:"lastMessageAt"`
		LastSenderName string    `json:"lastSenderName"`
		UnreadCount    int       `json:"unreadCount"`
	}
	var rooms []OwnerRoom
	for rows.Next() {
		var rm OwnerRoom
		if err := rows.Scan(&rm.RoomName, &rm.DeviceID, &rm.DeviceName,
			&rm.ImageUrl, &rm.RentPrice,
			&rm.LastMessage, &rm.LastMessageAt, &rm.LastSenderName, &rm.UnreadCount); err == nil {
			rooms = append(rooms, rm)
		}
	}
	if rooms == nil {
		rooms = []OwnerRoom{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": rooms})
}

// MarkNotificationsRead marks all (or specific) notifications as read.
// PATCH /api/chat/notifications/read   body: { "notifId": 5 } or {} for all
func (cc *ChatController) MarkNotificationsRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var body struct {
		NotifID int `json:"notifId"`
	}
	json.NewDecoder(r.Body).Decode(&body) // ignore parse error — body may be empty

	var err error
	if body.NotifID > 0 {
		_, err = cc.db.Exec(
			`UPDATE ChatNotification SET IsRead=true WHERE NotifId=$1 AND OwnerId=$2`,
			body.NotifID, userCtx.UserId,
		)
	} else {
		_, err = cc.db.Exec(
			`UPDATE ChatNotification SET IsRead=true WHERE OwnerId=$1 AND IsRead=false`,
			userCtx.UserId,
		)
	}
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
