-- Migration 012: Add sub-rating columns to UserReview and Review tables
-- Purpose: Support deeper per-criterion ratings beyond a single overall score.
--
--  UserReview (user-to-user):
--    RatingCommunication  – ทั้งสองฝ่าย: การสื่อสาร
--    RatingPunctuality    – ทั้งสองฝ่าย: ตรงเวลา
--    RatingAccuracy       – renter → owner only: ตรงตามที่บอก
--    RatingCare           – owner → renter only: ดูแลอุปกรณ์
--
--  Review (device review by renter):
--    RatingCondition      – สภาพอุปกรณ์ตามที่บอก
--    RatingValue          – คุ้มค่ากับราคา

BEGIN;

-- ── UserReview sub-ratings ─────────────────────────────────────────────────
ALTER TABLE UserReview
    ADD COLUMN IF NOT EXISTS RatingCommunication INTEGER CHECK (RatingCommunication BETWEEN 1 AND 5),
    ADD COLUMN IF NOT EXISTS RatingPunctuality   INTEGER CHECK (RatingPunctuality   BETWEEN 1 AND 5),
    ADD COLUMN IF NOT EXISTS RatingAccuracy      INTEGER CHECK (RatingAccuracy      BETWEEN 1 AND 5),
    ADD COLUMN IF NOT EXISTS RatingCare          INTEGER CHECK (RatingCare          BETWEEN 1 AND 5);

-- ── Review (device) sub-ratings ────────────────────────────────────────────
ALTER TABLE Review
    ADD COLUMN IF NOT EXISTS RatingCondition     INTEGER CHECK (RatingCondition BETWEEN 1 AND 5),
    ADD COLUMN IF NOT EXISTS RatingValue         INTEGER CHECK (RatingValue     BETWEEN 1 AND 5);

COMMIT;
