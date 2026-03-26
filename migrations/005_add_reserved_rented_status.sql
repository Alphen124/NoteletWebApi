-- Migration 005: Add Reserved and Rented status values
-- Created: 2026-03-16
-- Description: Add device status values for Reserved (booking confirmed, not yet started)
--              and Rented (rental actively ongoing) to replace the ambiguous
--              Delivered (2) / Returned (3) mapping used by the Chat confirm flow.

BEGIN;

-- Add new status entries (safe to re-run)
INSERT INTO Status (StatusNo, Name) VALUES
    (5, 'Reserved'),
    (6, 'Rented')
ON CONFLICT (StatusNo) DO NOTHING;

COMMIT;

-- Verification:
-- SELECT * FROM Status ORDER BY StatusNo;
