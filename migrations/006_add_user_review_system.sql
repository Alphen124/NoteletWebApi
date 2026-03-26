-- Migration 006: Add User-to-User Review System
-- Created: 2026-03-17
-- Description: Adds UserReview table for C2C peer reviews (renter ↔ owner)
--              linked to RentalRequest transactions with role-based conditions.
--              Also caches AvgRating on Owner/Renter tables via trigger.

BEGIN;

-- ─────────────────────────────────────────────────────────────────────────────
-- 1. UserReview table
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS UserReview (
    ReviewNo        SERIAL PRIMARY KEY,

    -- อ้างอิงรายการเช่า (1 transaction = 1 review per role)
    RequestNo       INTEGER NOT NULL,

    -- ผู้รีวิว / ผู้ถูกรีวิว
    ReviewerUserId  INTEGER NOT NULL,
    RevieweeUserId  INTEGER NOT NULL,

    -- บทบาทของผู้รีวิวในรายการนั้น
    ReviewerRole    VARCHAR(10) NOT NULL
                    CHECK (ReviewerRole IN ('renter', 'owner')),

    Rating          INTEGER NOT NULL CHECK (Rating BETWEEN 1 AND 5),
    Description     TEXT,

    -- Reply from reviewee (one reply only)
    ReplyText       TEXT,
    RepliedAt       TIMESTAMP,

    CreatedAt       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UpdatedAt       TIMESTAMP,

    -- Foreign keys
    CONSTRAINT fk_ur_request
        FOREIGN KEY (RequestNo)  REFERENCES RentalRequest(RequestNo) ON DELETE CASCADE,
    CONSTRAINT fk_ur_reviewer
        FOREIGN KEY (ReviewerUserId) REFERENCES AppUser(UserId) ON DELETE CASCADE,
    CONSTRAINT fk_ur_reviewee
        FOREIGN KEY (RevieweeUserId) REFERENCES AppUser(UserId) ON DELETE CASCADE,

    -- 1 transaction = max 1 review per role (blocks duplicate reviews)
    CONSTRAINT uq_ur_request_role UNIQUE (RequestNo, ReviewerRole)
);

CREATE INDEX IF NOT EXISTS idx_ur_reviewee  ON UserReview(RevieweeUserId);
CREATE INDEX IF NOT EXISTS idx_ur_reviewer  ON UserReview(ReviewerUserId);
CREATE INDEX IF NOT EXISTS idx_ur_requestno ON UserReview(RequestNo);

-- ─────────────────────────────────────────────────────────────────────────────
-- 2. Cache AvgRating on Owner/Renter tables
-- ─────────────────────────────────────────────────────────────────────────────
ALTER TABLE Owner  ADD COLUMN IF NOT EXISTS AvgRating NUMERIC(3,2) DEFAULT 0;
ALTER TABLE Renter ADD COLUMN IF NOT EXISTS AvgRating NUMERIC(3,2) DEFAULT 0;

-- ─────────────────────────────────────────────────────────────────────────────
-- 3. Trigger: auto-update AvgRating cache on INSERT / UPDATE / DELETE
-- ─────────────────────────────────────────────────────────────────────────────
CREATE OR REPLACE FUNCTION fn_update_user_avg_rating()
RETURNS TRIGGER AS $$
DECLARE
    v_target_id INTEGER;
    v_role      VARCHAR(10);
    v_avg       NUMERIC(3,2);
BEGIN
    v_target_id := COALESCE(NEW.RevieweeUserId, OLD.RevieweeUserId);
    v_role      := COALESCE(NEW.ReviewerRole,   OLD.ReviewerRole);

    SELECT ROUND(COALESCE(AVG(Rating), 0)::NUMERIC, 2)
    INTO v_avg
    FROM UserReview
    WHERE RevieweeUserId = v_target_id
      AND ReviewerRole   = v_role;

    -- ReviewerRole = 'renter'  → renter reviewed owner  → update Owner.AvgRating
    -- ReviewerRole = 'owner'   → owner reviewed renter  → update Renter.AvgRating
    IF v_role = 'renter' THEN
        UPDATE Owner SET AvgRating = v_avg WHERE UserId = v_target_id;
    ELSIF v_role = 'owner' THEN
        UPDATE Renter SET AvgRating = v_avg WHERE UserId = v_target_id;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_update_user_avg_rating ON UserReview;
CREATE TRIGGER trg_update_user_avg_rating
AFTER INSERT OR UPDATE OR DELETE ON UserReview
FOR EACH ROW EXECUTE FUNCTION fn_update_user_avg_rating();

COMMIT;

-- Verification queries (uncomment to run):
-- SELECT * FROM UserReview ORDER BY CreatedAt DESC LIMIT 10;
-- SELECT RevieweeUserId, ReviewerRole, ROUND(AVG(Rating),2) FROM UserReview GROUP BY 1,2;
