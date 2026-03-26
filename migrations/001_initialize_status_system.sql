-- Migration: Initialize Device Status System
-- Created: 2026-02-18
-- Description: Convert Device.Status column from VARCHAR to INTEGER and setup status tracking

BEGIN;

-- 1. Drop old constraint
ALTER TABLE Device DROP CONSTRAINT IF EXISTS device_status_check;

-- 2. Create Status table if not exists
CREATE TABLE IF NOT EXISTS Status (
    StatusNo INTEGER PRIMARY KEY,
    Name VARCHAR(50) NOT NULL UNIQUE
);

-- 3. Insert status values
INSERT INTO Status (StatusNo, Name) VALUES
    (1, 'Available'),
    (2, 'Delivered'),
    (3, 'Returned'),
    (4, 'Overdue')
ON CONFLICT (StatusNo) DO NOTHING;

-- 4. Update existing device statuses (convert string to integer)
UPDATE Device SET Status = '1' WHERE Status = 'available' OR Status IS NULL;
UPDATE Device SET Status = '2' WHERE Status = 'rented';

-- 5. Modify Device.Status column type
ALTER TABLE Device ALTER COLUMN Status DROP DEFAULT;
ALTER TABLE Device ALTER COLUMN Status TYPE INTEGER USING Status::integer;
ALTER TABLE Device ALTER COLUMN Status SET DEFAULT 1;

-- 6. Add foreign key constraint
ALTER TABLE Device 
ADD CONSTRAINT fk_device_status 
FOREIGN KEY (Status) REFERENCES Status(StatusNo);

-- 7. Create DeviceStatusHistory table
CREATE TABLE IF NOT EXISTS DeviceStatusHistory (
    HistoryNo SERIAL PRIMARY KEY,
    DeviceNo INTEGER NOT NULL,
    StatusNo INTEGER NOT NULL,
    ChangedBy INTEGER NOT NULL,
    ChangedAt TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (DeviceNo) REFERENCES Device(DeviceNo) ON DELETE CASCADE,
    FOREIGN KEY (StatusNo) REFERENCES Status(StatusNo),
    FOREIGN KEY (ChangedBy) REFERENCES AppUser(UserId)
);

-- 8. Create index for performance
CREATE INDEX IF NOT EXISTS idx_device_status ON Device(Status);
CREATE INDEX IF NOT EXISTS idx_status_history_device ON DeviceStatusHistory(DeviceNo);
CREATE INDEX IF NOT EXISTS idx_status_history_date ON DeviceStatusHistory(ChangedAt DESC);

COMMIT;

-- Verification queries (uncomment to run):
-- SELECT * FROM Status ORDER BY StatusNo;
-- SELECT DeviceNo, DeviceName, Status FROM Device;
-- SELECT COUNT(*) FROM DeviceStatusHistory;
