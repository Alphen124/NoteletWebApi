-- Migration 002: Add Review table for device reviews with ratings and descriptions

CREATE TABLE IF NOT EXISTS Review (
    ReviewNo        SERIAL PRIMARY KEY,
    DeviceNo        INTEGER NOT NULL,
    ReviewerUserId  INTEGER NOT NULL,
    Rating          INTEGER NOT NULL CHECK (Rating BETWEEN 1 AND 5),
    Description     TEXT,
    CreatedAt       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_review_device
        FOREIGN KEY (DeviceNo)
        REFERENCES Device(DeviceNo) ON DELETE CASCADE,
    CONSTRAINT fk_review_user
        FOREIGN KEY (ReviewerUserId)
        REFERENCES AppUser(UserId) ON DELETE CASCADE,
    CONSTRAINT uq_review_user_device
        UNIQUE (DeviceNo, ReviewerUserId)
);

-- Index for fast lookup by device
CREATE INDEX IF NOT EXISTS idx_review_deviceno ON Review(DeviceNo);

-- Trigger to update Device.Rating when a review is added/updated/deleted
CREATE OR REPLACE FUNCTION fn_update_device_rating_from_reviews()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE Device
    SET Rating = (
        SELECT COALESCE(AVG(Rating::NUMERIC), 0)
        FROM Review
        WHERE DeviceNo = COALESCE(NEW.DeviceNo, OLD.DeviceNo)
    )
    WHERE DeviceNo = COALESCE(NEW.DeviceNo, OLD.DeviceNo);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_update_device_rating_from_reviews ON Review;
CREATE TRIGGER trg_update_device_rating_from_reviews
AFTER INSERT OR UPDATE OR DELETE ON Review
FOR EACH ROW
EXECUTE FUNCTION fn_update_device_rating_from_reviews();
