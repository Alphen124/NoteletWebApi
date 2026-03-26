-- Migration 003: Add Chat tables and ChatNotification for lender alerts
-- Created: 2026-03-14

BEGIN;

-- ── Chat Room ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS ChatRoom (
    RoomId    SERIAL PRIMARY KEY,
    RoomName  VARCHAR(100) NOT NULL UNIQUE,
    IsPublic  BOOLEAN DEFAULT FALSE,
    DeviceId  INTEGER,
    CreatedAt TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_chatroom_device
        FOREIGN KEY (DeviceId) REFERENCES Device(DeviceNo) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_chatroom_name   ON ChatRoom(RoomName);
CREATE INDEX IF NOT EXISTS idx_chatroom_device ON ChatRoom(DeviceId);

-- ── Chat Message ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS ChatMessage (
    MessageId SERIAL PRIMARY KEY,
    RoomId    INTEGER NOT NULL,
    SenderId  INTEGER NOT NULL,
    Content   TEXT NOT NULL,
    CreatedAt TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_chatmsg_room
        FOREIGN KEY (RoomId)   REFERENCES ChatRoom(RoomId) ON DELETE CASCADE,
    CONSTRAINT fk_chatmsg_user
        FOREIGN KEY (SenderId) REFERENCES AppUser(UserId)  ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_chatmsg_room_ts ON ChatMessage(RoomId, CreatedAt DESC);

-- ── Chat Notification (sent to device owner when renter starts chatting) ─────
CREATE TABLE IF NOT EXISTS ChatNotification (
    NotifId    SERIAL PRIMARY KEY,
    OwnerId    INTEGER NOT NULL,
    RoomId     INTEGER NOT NULL,
    DeviceId   INTEGER,
    DeviceName VARCHAR(100),
    SenderName VARCHAR(100),
    Preview    TEXT,
    IsRead     BOOLEAN DEFAULT FALSE,
    CreatedAt  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_chatnotif_owner
        FOREIGN KEY (OwnerId) REFERENCES AppUser(UserId) ON DELETE CASCADE,
    CONSTRAINT fk_chatnotif_room
        FOREIGN KEY (RoomId)  REFERENCES ChatRoom(RoomId) ON DELETE CASCADE,
    CONSTRAINT fk_chatnotif_device
        FOREIGN KEY (DeviceId) REFERENCES Device(DeviceNo) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_chatnotif_owner ON ChatNotification(OwnerId, IsRead, CreatedAt DESC);

-- Seed a public "general" room so the hub has somewhere to fall back
INSERT INTO ChatRoom (RoomName, IsPublic) VALUES ('general', true)
ON CONFLICT (RoomName) DO NOTHING;

COMMIT;
