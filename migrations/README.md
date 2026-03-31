# migrations/

SQL migration scripts สำหรับสร้างและปรับโครงสร้างฐานข้อมูล NoteLet

## 📋 ไฟล์ทั้งหมด

| ไฟล์ | วันที่ | สิ่งที่เปลี่ยนแปลง |
|---|---|---|
| `001_initialize_status_system.sql` | 2026-02-18 | แปลง `Device.Status` จาก VARCHAR เป็น INTEGER, สร้างตาราง `Status` และ `DeviceStatusHistory` |
| `002_add_review_table.sql` | 2026-02-xx | สร้างตาราง `Review` สำหรับรีวิวอุปกรณ์ (rating 1–5) |
| `003_add_chat_and_notifications.sql` | 2026-03-14 | สร้างตาราง `ChatRoom`, `ChatMessage`, `ChatNotification` |
| `004_rename_rental_statuses.sql` | 2026-03-16 | เปลี่ยนชื่อ status ใน `RentalRequest` ให้ตรงกับ lifecycle labels |
| `005_add_reserved_rented_status.sql` | 2026-03-16 | เพิ่มค่า status 2 (Booking Confirmed) และ 3 (Rental Active) ในตาราง `Status` |
| `006_add_user_review_system.sql` | 2026-03-17 | สร้างตาราง `UserReview`, เพิ่ม `AvgRating` ใน Owner/Renter, trigger คำนวณคะแนน |
| `007_convert_pk_to_serial.sql` | 2026-03-17 | แปลง PK ของ `RentBill`, `Schedule`, `StatusHistory` เป็น SERIAL |
| `008_fix_rentbill_rating_triggers.sql` | 2026-03-17 | แก้ trigger `fn_update_owner_rating` / `fn_update_renter_rating` ให้ชี้ `AvgRating` แทน `Rating` |
| `009_add_admin_system.sql` | 2026-03-xx | เพิ่ม `is_admin` ใน `appuser` และ `is_admin_device` ใน `Device` |

---

## รายละเอียดแต่ละ Migration

### 001 — Initialize Status System
- ลบ constraint `device_status_check` เก่า
- สร้างตาราง `Status` (StatusNo, Name)
- สร้างตาราง `DeviceStatusHistory`
- แปลง `Device.Status` VARCHAR → INTEGER FK
- เพิ่ม indexes

### 002 — Add Review Table
```sql
CREATE TABLE Review (
    ReviewNo        SERIAL PRIMARY KEY,
    DeviceNo        INTEGER NOT NULL,
    ReviewerUserId  INTEGER NOT NULL,
    Rating          INTEGER NOT NULL CHECK (Rating BETWEEN 1 AND 5),
    Description     TEXT,
    CreatedAt       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (DeviceNo, ReviewerUserId)  -- 1 รีวิว/user/อุปกรณ์
)
```

### 003 — Add Chat & Notifications
- `ChatRoom` (RoomId, RoomName, IsPublic)
- `ChatMessage` (ข้อความ + metadata)
- `ChatNotification` (การแจ้งเตือนสำหรับเจ้าของ)

### 004 — Rename Rental Statuses
เปลี่ยนชื่อ status ใน `RentalRequest`:
```
Pending   → Request Pending
Confirmed → Booking Confirmed
Active    → Rental Active
Completed → Rental Completed
```

### 005 — Add Reserved/Rented Status
เพิ่มค่าใหม่ในตาราง `Status`:
```sql
INSERT INTO Status VALUES (2, 'Booking Confirmed'), (3, 'Rental Active');
```

### 006 — User Review System
- สร้างตาราง `UserReview` (รีวิว renter ↔ owner)
- เพิ่ม column `AvgRating` ในตาราง `Owner` และ `Renter`
- Trigger คำนวณค่า `AvgRating` อัตโนมัติหลัง INSERT/UPDATE review

### 007 — Convert PK to SERIAL
แก้ปัญหา `INSERT` ล้มเมื่อไม่ใส่ค่า PK:
- `RentBill.RentingNo` → SERIAL
- `Schedule.ScheduleNo` → SERIAL
- `StatusHistory.HistoryNo` → SERIAL

### 008 — Fix RentBill Rating Triggers
แก้ bug: trigger `fn_update_owner_rating` / `fn_update_renter_rating`
ยังอ้างอิง column `Rating` (เก่า) แทนที่จะเป็น `AvgRating` (ใหม่)
ทำให้ทุก INSERT ใน `RentBill` ล้มด้วย `HTTP 500`

### 009 — Add Admin System
```sql
ALTER TABLE appuser ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE Device  ADD COLUMN IF NOT EXISTS is_admin_device BOOLEAN NOT NULL DEFAULT FALSE;
```

---

## วิธีรัน Migration

```bash
# รันผ่าน psql โดยตรง
psql -U postgres -d notelet_db -f migrations/001_initialize_status_system.sql

# รันผ่าน Docker
docker exec -i <container> psql -U alphen -d notelet < migrations/001_initialize_status_system.sql
```

> หมายเหตุ: `main.go` รัน migration บางส่วนอัตโนมัติเมื่อเริ่มต้น server (สร้างตาราง `Review`, trigger `Device.Rating`, ตาราง `RentalRequest`)

## Naming Convention

```
XXX_description_of_migration.sql
```
- `XXX` = เลขลำดับเริ่มต้นจาก 001
- `description` = Brief description using underscores
- Always use `.sql` extension

## Current Migrations

### 001_initialize_status_system.sql
**Date:** 2026-02-18  
**Purpose:** Initialize device status tracking system
- Convert Device.Status from VARCHAR to INTEGER
- Create Status table (1-4)
- Create DeviceStatusHistory table
- Add foreign key constraints
- Add performance indexes

## How to Run Migrations

### Manual Execution
```bash
docker exec -i postgres psql -U alphen -d notelet < migrations/001_initialize_status_system.sql
```

### Verify Migration
```sql
-- Check Status table
SELECT * FROM Status ORDER BY StatusNo;

-- Check Device statuses
SELECT DeviceNo, DeviceName, Status FROM Device;

-- Check status history
SELECT COUNT(*) FROM DeviceStatusHistory;
```

## Migration Guidelines

### Before Creating New Migration:
1. Test on development database first
2. Use transactions (BEGIN/COMMIT)
3. Include rollback instructions
4. Add comments explaining changes
5. Use `IF EXISTS` / `IF NOT EXISTS` for safety

### Migration Template:
```sql
-- Migration: [Title]
-- Created: [YYYY-MM-DD]
-- Description: [What this migration does]

BEGIN;

-- Your SQL statements here
ALTER TABLE ...;

COMMIT;

-- Rollback (comment out):
-- BEGIN;
-- [Reverse operations]
-- COMMIT;
```

## Rollback Procedures

### 001_initialize_status_system.sql Rollback:
```sql
BEGIN;

-- Drop new tables
DROP TABLE IF EXISTS DeviceStatusHistory CASCADE;
DROP TABLE IF EXISTS Status CASCADE;

-- Revert Device.Status to VARCHAR
ALTER TABLE Device ALTER COLUMN Status DROP DEFAULT;
ALTER TABLE Device ALTER COLUMN Status TYPE VARCHAR(20);
ALTER TABLE Device ALTER COLUMN Status SET DEFAULT 'available';
ALTER TABLE Device 
ADD CONSTRAINT device_status_check 
CHECK (Status IN ('available', 'rented'));

COMMIT;
```

## Migration Status

| # | File | Status | Date Applied |
|---|------|--------|--------------|
| 001 | initialize_status_system | ✅ Applied | 2026-02-18 |

## Notes
- Always backup database before running migrations
- Test migrations on staging environment first
- Document any manual steps required
- Keep migrations idempotent when possible
