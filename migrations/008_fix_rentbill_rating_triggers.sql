-- Migration 008: Fix RentBill rating triggers to reference AvgRating
-- Created: 2026-03-17
-- Description: Migration 006 added AvgRating to Owner and Renter tables but did NOT
--              update the trigger functions fn_update_owner_rating() and
--              fn_update_renter_rating() which still referenced the old Rating column.
--              This caused every INSERT into RentBill to fail with:
--                ERROR: column "rating" of relation "owner" does not exist
--              which surfaced as HTTP 500 "Failed to create rent bill" when attempting
--              to confirm a rental from the Chat page.

BEGIN;

-- Fix fn_update_owner_rating: use AvgRating instead of Rating
CREATE OR REPLACE FUNCTION fn_update_owner_rating()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    UPDATE Owner o
    SET AvgRating = (
        SELECT COALESCE(AVG(rb.Rating), 0)
        FROM RentBill rb
        JOIN RentList rl ON rb.RentingNo = rl.RentingNo
        JOIN DeviceOwner do2 ON rl.DeviceNo = do2.DeviceNo
        WHERE do2.OwnerNo = o.OwnerNo
    )
    WHERE o.OwnerNo IN (
        SELECT do2.OwnerNo
        FROM RentList rl
        JOIN DeviceOwner do2 ON rl.DeviceNo = do2.DeviceNo
        WHERE rl.RentingNo = NEW.RentingNo
    );
    RETURN NULL;
END;
$$;

-- Fix fn_update_renter_rating: use AvgRating instead of Rating
CREATE OR REPLACE FUNCTION fn_update_renter_rating()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    UPDATE Renter r
    SET AvgRating = (
        SELECT COALESCE(AVG(Rating), 0)
        FROM RentBill
        WHERE RenterNo = NEW.RenterNo
    )
    WHERE r.RenterNo = NEW.RenterNo;
    RETURN NULL;
END;
$$;

COMMIT;

-- Verification:
-- SELECT proname, prosrc FROM pg_proc WHERE proname IN ('fn_update_owner_rating','fn_update_renter_rating');
-- The prosrc should reference 'AvgRating' not 'Rating' for Owner and Renter updates.
