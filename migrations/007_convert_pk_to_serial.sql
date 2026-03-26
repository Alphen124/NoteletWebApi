-- Migration 007: Convert INTEGER PRIMARY KEY columns to SERIAL (auto-increment)
-- Affected tables: RentBill, Schedule, StatusHistory
-- These tables had plain INTEGER PKs which caused INSERT failures when no value was supplied.

BEGIN;

-- ── RentBill.RentingNo ────────────────────────────────────────────────────
-- Create a sequence seeded to MAX+1 (or 1 if empty) and attach it as the default
CREATE SEQUENCE IF NOT EXISTS rentbill_rentingno_seq
    START 1 INCREMENT 1;

SELECT setval(
    'rentbill_rentingno_seq',
    COALESCE((SELECT MAX(RentingNo) FROM RentBill), 0) + 1,
    false   -- false = next call returns this value exactly
);

ALTER TABLE RentBill
    ALTER COLUMN RentingNo SET DEFAULT nextval('rentbill_rentingno_seq');

ALTER SEQUENCE rentbill_rentingno_seq OWNED BY RentBill.RentingNo;

-- ── Schedule.ScheduleNo ──────────────────────────────────────────────────
CREATE SEQUENCE IF NOT EXISTS schedule_scheduleno_seq
    START 1 INCREMENT 1;

SELECT setval(
    'schedule_scheduleno_seq',
    COALESCE((SELECT MAX(ScheduleNo) FROM Schedule), 0) + 1,
    false
);

ALTER TABLE Schedule
    ALTER COLUMN ScheduleNo SET DEFAULT nextval('schedule_scheduleno_seq');

ALTER SEQUENCE schedule_scheduleno_seq OWNED BY Schedule.ScheduleNo;

-- ── StatusHistory.HistoryNo ──────────────────────────────────────────────
CREATE SEQUENCE IF NOT EXISTS statushistory_historyno_seq
    START 1 INCREMENT 1;

SELECT setval(
    'statushistory_historyno_seq',
    COALESCE((SELECT MAX(HistoryNo) FROM StatusHistory), 0) + 1,
    false
);

ALTER TABLE StatusHistory
    ALTER COLUMN HistoryNo SET DEFAULT nextval('statushistory_historyno_seq');

ALTER SEQUENCE statushistory_historyno_seq OWNED BY StatusHistory.HistoryNo;

COMMIT;

-- Verification:
-- SELECT column_name, column_default FROM information_schema.columns
-- WHERE table_name IN ('rentbill','schedule','statushistory')
--   AND column_name IN ('rentingno','scheduleno','historyno');
