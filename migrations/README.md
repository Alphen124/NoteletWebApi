# Database Migrations

## Overview
This folder contains SQL migration scripts for the Notelet database schema.

## Naming Convention
```
XXX_description_of_migration.sql
```
- `XXX` = Sequential number (001, 002, etc.)
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
