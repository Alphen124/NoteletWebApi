-- Migration: Add is_central_staff column to appuser
-- Run once against your PostgreSQL database

ALTER TABLE appuser ADD COLUMN IF NOT EXISTS is_central_staff BOOLEAN NOT NULL DEFAULT false;

-- Create the central staff account
-- Email: staff@notelet.com  |  Password: StaffNotelet@2026
-- Hash below is bcrypt cost-10 for "StaffNotelet@2026"
INSERT INTO appuser (email, passwordhash, isactive, is_admin, is_central_staff, createdat)
VALUES (
    'staff@notelet.com',
    '$2a$10$9QGkRFv/M.I6c8dE2F3B4.kFxIVe8kT7bRqFNyqTnOW3F0Ywr2/Xe',
    true,
    false,
    true,
    NOW()
)
ON CONFLICT (email) DO UPDATE SET is_central_staff = true;

-- Ensure Owner + Renter profiles exist for staff account
DO $$
DECLARE
  v_user_id  INT;
  v_owner_no INT;
  v_renter_no INT;
BEGIN
  SELECT userid INTO v_user_id FROM appuser WHERE email = 'staff@notelet.com';

  IF NOT EXISTS (SELECT 1 FROM owner WHERE userid = v_user_id) THEN
    SELECT COALESCE(MAX(ownerno), 0) + 1 INTO v_owner_no FROM owner;
    INSERT INTO owner (ownerno, name, fname, lname, tel, userid)
    VALUES (v_owner_no, 'เจ้าหน้าที่ ส่วนกลาง', 'เจ้าหน้าที่', 'ส่วนกลาง', '0800000000', v_user_id);
  END IF;

  IF NOT EXISTS (SELECT 1 FROM renter WHERE userid = v_user_id) THEN
    SELECT COALESCE(MAX(renterno), 0) + 1 INTO v_renter_no FROM renter;
    INSERT INTO renter (renterno, name, fname, lname, tel, userid)
    VALUES (v_renter_no, 'เจ้าหน้าที่ ส่วนกลาง', 'เจ้าหน้าที่', 'ส่วนกลาง', '0800000000', v_user_id);
  END IF;
END $$;
