-- Migration: Rename RentalRequest status values
-- Created: 2026-03-16
-- Description: Rename status values in RentalRequest to match the 4-step lifecycle labels

BEGIN;

-- 1. Drop the existing CHECK constraint
ALTER TABLE RentalRequest DROP CONSTRAINT IF EXISTS rentalrequest_status_check;

-- 2. Rename existing data
UPDATE RentalRequest SET Status = 'Request Pending'  WHERE Status = 'pending';
UPDATE RentalRequest SET Status = 'Booking Confirmed' WHERE Status = 'confirmed';
UPDATE RentalRequest SET Status = 'Rental Completed'  WHERE Status = 'returned';

-- 3. Add updated CHECK constraint with new status names (including Rental Active)
ALTER TABLE RentalRequest
    ADD CONSTRAINT rentalrequest_status_check
    CHECK (Status IN (
        'Request Pending',
        'Booking Confirmed',
        'Rental Active',
        'Rental Completed',
        'rejected',
        'cancelled'
    ));

-- 4. Update the column default
ALTER TABLE RentalRequest ALTER COLUMN Status SET DEFAULT 'Request Pending';

COMMIT;

-- Verification:
-- SELECT Status, COUNT(*) FROM RentalRequest GROUP BY Status ORDER BY Status;
