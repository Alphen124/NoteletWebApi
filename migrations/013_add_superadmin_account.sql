-- Migration 013: Add superadmin account
-- Email: superadmin@notelet.com  |  Password: Admin@Notelet2026
-- bcrypt cost-10 hash of "Admin@Notelet2026"

INSERT INTO appuser (email, passwordhash, isactive, is_admin, is_central_staff, createdat)
VALUES (
    'superadmin@notelet.com',
    '$2a$10$2b7XCxTrLKmpfE.jUSHive.xl27m5z6EmdFNaZOJV5CPXThWTo7Ku',
    true,
    true,
    true,
    NOW()
)
ON CONFLICT (email) DO UPDATE SET is_admin = true, is_central_staff = true;

-- Ensure Owner + Renter profiles exist for superadmin
DO $$
DECLARE
  v_user_id   INT;
  v_owner_no  INT;
  v_renter_no INT;
BEGIN
  SELECT userid INTO v_user_id FROM appuser WHERE email = 'superadmin@notelet.com';

  IF NOT EXISTS (SELECT 1 FROM owner WHERE userid = v_user_id) THEN
    SELECT COALESCE(MAX(ownerno), 0) + 1 INTO v_owner_no FROM owner;
    INSERT INTO owner (ownerno, name, fname, lname, tel, userid)
    VALUES (v_owner_no, 'ผู้ดูแลระบบ Notelet', 'ผู้ดูแล', 'ระบบ', '0800000000', v_user_id);
  END IF;

  IF NOT EXISTS (SELECT 1 FROM renter WHERE userid = v_user_id) THEN
    SELECT COALESCE(MAX(renterno), 0) + 1 INTO v_renter_no FROM renter;
    INSERT INTO renter (renterno, name, fname, lname, tel, userid)
    VALUES (v_renter_no, 'ผู้ดูแลระบบ Notelet', 'ผู้ดูแล', 'ระบบ', '0800000000', v_user_id);
  END IF;
END $$;
