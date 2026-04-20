package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	database "noteletwebservice-development/config/database"
	"noteletwebservice-development/controllers"
	"noteletwebservice-development/routers"
	firebasesvc "noteletwebservice-development/services/firebase"
	"noteletwebservice-development/services/oauth"
	"noteletwebservice-development/utils"

	"github.com/joho/godotenv"
)

// Notelet Backend API
// REST API server for device rental system
// Runs on port 3001 and provides API endpoints with database persistence
func main() {
	// ============================================================
	// STEP 1: Load Configuration
	// ============================================================
	// โหลด environment variables จาก .env (ถ้ามี)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// ============================================================
	// STEP 2: Initialize Database Connection
	// ============================================================
	// เชื่อมต่อฐานข้อมูล PostgreSQL
	db := database.ConnectNoteletDB()
	defer db.Close()

	fmt.Println("✓ Successfully connected to database!")

	// ============================================================
	// STEP 3: Initialize External Services (OAuth)
	// ============================================================
	// ตั้งค่า Google OAuth สำหรับการล็อกอิน
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	redirectURL := os.Getenv("GOOGLE_REDIRECT_URL")
	if redirectURL == "" {
		redirectURL = "http://localhost:3001/api/auth/google/callback"
	}

	// ============================================================
	// STEP 4: Run Database Migrations
	// ============================================================
	oauth.InitGoogleOAuth(clientID, clientSecret, redirectURL)
	fmt.Println("✓ Google OAuth initialized")

	// Initialise Firebase Admin SDK (for Google Sign-In via Firebase)
	if err := firebasesvc.Init(); err != nil {
		log.Printf("Warning: Firebase Admin SDK not initialised (%v) — /api/auth/firebase will not work until serviceAccountKey.json is added", err)
	} else {
		fmt.Println("✓ Firebase Admin SDK initialized")
	}

	// Run Review table migration at startup
	migrationSQL := `CREATE TABLE IF NOT EXISTS Review (
		ReviewNo        SERIAL PRIMARY KEY,
		DeviceNo        INTEGER NOT NULL,
		ReviewerUserId  INTEGER NOT NULL,
		Rating          INTEGER NOT NULL CHECK (Rating BETWEEN 1 AND 5),
		Description     TEXT,
		CreatedAt       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT fk_review_device FOREIGN KEY (DeviceNo) REFERENCES Device(DeviceNo) ON DELETE CASCADE,
		CONSTRAINT fk_review_user FOREIGN KEY (ReviewerUserId) REFERENCES AppUser(UserId) ON DELETE CASCADE,
		CONSTRAINT uq_review_user_device UNIQUE (DeviceNo, ReviewerUserId)
	);
	CREATE INDEX IF NOT EXISTS idx_review_deviceno ON Review(DeviceNo);`
	if _, err := db.Exec(migrationSQL); err != nil {
		fmt.Printf("Warning: Review migration error (table may already exist): %v\n", err)
	} else {
		fmt.Println("\u2713 Review table ready")
	}

	// Ensure Device.Rating trigger is installed (updates Device.Rating = AVG of Review.Rating)
	deviceRatingTriggerSQL := `
		CREATE OR REPLACE FUNCTION fn_update_device_rating_from_reviews()
		RETURNS TRIGGER AS $$
		BEGIN
			UPDATE Device
			SET Rating = (
				SELECT COALESCE(AVG(r.Rating::NUMERIC), 0)
				FROM Review r
				WHERE r.DeviceNo = COALESCE(NEW.DeviceNo, OLD.DeviceNo)
			)
			WHERE DeviceNo = COALESCE(NEW.DeviceNo, OLD.DeviceNo);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		DROP TRIGGER IF EXISTS trg_update_device_rating_from_reviews ON Review;
		CREATE TRIGGER trg_update_device_rating_from_reviews
		AFTER INSERT OR UPDATE OR DELETE ON Review
		FOR EACH ROW EXECUTE FUNCTION fn_update_device_rating_from_reviews();
	`
	if _, err := db.Exec(deviceRatingTriggerSQL); err != nil {
		fmt.Printf("Warning: Device rating trigger error: %v\n", err)
	} else {
		fmt.Println("\u2713 Device rating trigger ready")
	}

	// Run RentalRequest table migration at startup
	rentalMigrationSQL := `CREATE TABLE IF NOT EXISTS RentalRequest (
		RequestNo    SERIAL PRIMARY KEY,
		DeviceNo     INTEGER NOT NULL,
		RenterUserId INTEGER NOT NULL,
		StartDate    DATE NOT NULL,
		EndDate      DATE NOT NULL,
		TotalPrice   NUMERIC(10,2) NOT NULL DEFAULT 0,
		Note         TEXT,
		Status       VARCHAR(50) NOT NULL DEFAULT 'Request Pending'
		             CHECK (Status IN ('Request Pending','Booking Confirmed','Rental Active','Rental Completed','rejected','cancelled')),
		ScheduleNo   INTEGER,
		RentingNo    INTEGER,
		RentListNo   INTEGER,
		RentListSeq  INTEGER,
		CreatedAt    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UpdatedAt    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT fk_rr_device FOREIGN KEY (DeviceNo) REFERENCES Device(DeviceNo) ON DELETE CASCADE,
		CONSTRAINT fk_rr_renter FOREIGN KEY (RenterUserId) REFERENCES AppUser(UserId) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_rr_deviceno ON RentalRequest(DeviceNo);
	CREATE INDEX IF NOT EXISTS idx_rr_renterid ON RentalRequest(RenterUserId);
	CREATE INDEX IF NOT EXISTS idx_rr_status ON RentalRequest(Status);
	ALTER TABLE RentalRequest ADD COLUMN IF NOT EXISTS ScheduleNo  INTEGER;
	ALTER TABLE RentalRequest ADD COLUMN IF NOT EXISTS RentingNo   INTEGER;
	ALTER TABLE RentalRequest ADD COLUMN IF NOT EXISTS RentListNo  INTEGER;
	ALTER TABLE RentalRequest ADD COLUMN IF NOT EXISTS RentListSeq INTEGER;
	ALTER TABLE RentalRequest DROP CONSTRAINT IF EXISTS rentalrequest_status_check;
	ALTER TABLE RentalRequest ADD CONSTRAINT rentalrequest_status_check
	    CHECK (Status IN ('Request Pending','Booking Confirmed','Rental Active','Rental Completed','rejected','cancelled'));
	ALTER TABLE RentalRequest ALTER COLUMN Status SET DEFAULT 'Request Pending';
	ALTER TABLE RentalRequest ADD COLUMN IF NOT EXISTS PickupTime TEXT;
	ALTER TABLE RentalRequest ADD COLUMN IF NOT EXISTS ReturnTime TEXT;`
	if _, err := db.Exec(rentalMigrationSQL); err != nil {
		fmt.Printf("Warning: RentalRequest migration error: %v\n", err)
	} else {
		fmt.Println("\u2713 RentalRequest table ready")
	}

	// Integration migration: add auto-increment sequences to legacy tables
	for _, stmt := range []string{
		`CREATE SEQUENCE IF NOT EXISTS seq_schedule_no`,
		`SELECT setval('seq_schedule_no', GREATEST(COALESCE((SELECT MAX(ScheduleNo) FROM Schedule), 0) + 1, 1), false)`,
		`ALTER TABLE Schedule ALTER COLUMN ScheduleNo SET DEFAULT nextval('seq_schedule_no')`,
		`CREATE SEQUENCE IF NOT EXISTS seq_rentbill_no`,
		`SELECT setval('seq_rentbill_no', GREATEST(COALESCE((SELECT MAX(RentingNo) FROM RentBill), 0) + 1, 1), false)`,
		`ALTER TABLE RentBill ALTER COLUMN RentingNo SET DEFAULT nextval('seq_rentbill_no')`,
		`CREATE SEQUENCE IF NOT EXISTS seq_rentlist_no`,
		`SELECT setval('seq_rentlist_no', GREATEST(COALESCE((SELECT MAX(RentListNo) FROM RentList), 0) + 1, 1), false)`,
		`CREATE SEQUENCE IF NOT EXISTS seq_renter_no`,
		`SELECT setval('seq_renter_no', GREATEST(COALESCE((SELECT MAX(RenterNo) FROM Renter), 0) + 1, 1), false)`,
		`ALTER TABLE Renter ALTER COLUMN RenterNo SET DEFAULT nextval('seq_renter_no')`,
		`CREATE SEQUENCE IF NOT EXISTS seq_statushistory_no`,
		`SELECT setval('seq_statushistory_no', GREATEST(COALESCE((SELECT MAX(HistoryNo) FROM StatusHistory), 0) + 1, 1), false)`,
		`ALTER TABLE StatusHistory ALTER COLUMN HistoryNo SET DEFAULT nextval('seq_statushistory_no')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			fmt.Printf("Warning: integration migration (%s...): %v\n", stmt[:40], err)
		}
	}
	fmt.Println("\u2713 Integration sequences ready")

	// Update insert_rentlist stored procedure to return generated IDs
	// Must DROP first because return type changes from VOID to TABLE
	if _, err := db.Exec(`DROP FUNCTION IF EXISTS insert_rentlist(INTEGER, INTEGER, INTEGER, INTEGER)`); err != nil {
		fmt.Printf("Warning: drop insert_rentlist: %v\n", err)
	}
	insertRentlistSQL := `CREATE OR REPLACE FUNCTION insert_rentlist(
		p_rentlistno  INTEGER,
		p_deviceno    INTEGER,
		p_scheduleno  INTEGER,
		p_rentingno   INTEGER
	)
	RETURNS TABLE(out_rentlistno INTEGER, out_seq INTEGER) AS $$
	DECLARE
		v_next_seq   INTEGER;
		v_rentlistno INTEGER;
	BEGIN
		IF p_rentlistno = 0 THEN
			SELECT nextval('seq_rentlist_no') INTO v_rentlistno;
		ELSE
			v_rentlistno := p_rentlistno;
		END IF;
		SELECT COALESCE(MAX(RentListSeq), 0) + 1
		INTO v_next_seq
		FROM RentList
		WHERE RentListNo = v_rentlistno;
		INSERT INTO RentList (RentListNo, RentListSeq, DeviceNo, ScheduleNo, RentingNo)
		VALUES (v_rentlistno, v_next_seq, p_deviceno, p_scheduleno, p_rentingno);
		RETURN QUERY SELECT v_rentlistno, v_next_seq;
	END;
	$$ LANGUAGE plpgsql`
	if _, err := db.Exec(insertRentlistSQL); err != nil {
		fmt.Printf("Warning: insert_rentlist procedure update error: %v\n", err)
	} else {
		fmt.Println("\u2713 insert_rentlist procedure ready")
	}

	// UserReview table migration (migration 006)
	userReviewMigrationSQL := `
		CREATE TABLE IF NOT EXISTS UserReview (
			ReviewNo        SERIAL PRIMARY KEY,
			RequestNo       INTEGER NOT NULL,
			ReviewerUserId  INTEGER NOT NULL,
			RevieweeUserId  INTEGER NOT NULL,
			ReviewerRole    VARCHAR(10) NOT NULL CHECK (ReviewerRole IN ('renter', 'owner')),
			Rating          INTEGER NOT NULL CHECK (Rating BETWEEN 1 AND 5),
			Description     TEXT,
			ReplyText       TEXT,
			RepliedAt       TIMESTAMP,
			CreatedAt       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UpdatedAt       TIMESTAMP,
			CONSTRAINT fk_ur_request  FOREIGN KEY (RequestNo)      REFERENCES RentalRequest(RequestNo) ON DELETE CASCADE,
			CONSTRAINT fk_ur_reviewer FOREIGN KEY (ReviewerUserId) REFERENCES AppUser(UserId) ON DELETE CASCADE,
			CONSTRAINT fk_ur_reviewee FOREIGN KEY (RevieweeUserId) REFERENCES AppUser(UserId) ON DELETE CASCADE,
			CONSTRAINT uq_ur_request_role UNIQUE (RequestNo, ReviewerRole)
		);
		CREATE INDEX IF NOT EXISTS idx_ur_reviewee  ON UserReview(RevieweeUserId);
		CREATE INDEX IF NOT EXISTS idx_ur_reviewer  ON UserReview(ReviewerUserId);
		CREATE INDEX IF NOT EXISTS idx_ur_requestno ON UserReview(RequestNo);
		ALTER TABLE Owner  ADD COLUMN IF NOT EXISTS AvgRating NUMERIC(3,2) DEFAULT 0;
		ALTER TABLE Renter ADD COLUMN IF NOT EXISTS AvgRating NUMERIC(3,2) DEFAULT 0;
	`
	if _, err := db.Exec(userReviewMigrationSQL); err != nil {
		fmt.Printf("Warning: UserReview migration error: %v\n", err)
	} else {
		fmt.Println("\u2713 UserReview table ready")
	}

	// UserReview trigger: auto-update AvgRating cache on Owner/Renter
	userReviewTriggerSQL := `
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
			WHERE RevieweeUserId = v_target_id AND ReviewerRole = v_role;
			IF v_role = 'renter' THEN
				UPDATE Owner  SET AvgRating = v_avg WHERE UserId = v_target_id;
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
	`
	if _, err := db.Exec(userReviewTriggerSQL); err != nil {
		fmt.Printf("Warning: UserReview trigger error: %v\n", err)
	} else {
		fmt.Println("\u2713 UserReview avg-rating trigger ready")
	}

	// Chat DB migration
	chatMigrationSQL := `
		CREATE TABLE IF NOT EXISTS ChatRoom (
			RoomId    SERIAL PRIMARY KEY,
			RoomName  VARCHAR(100) NOT NULL UNIQUE,
			IsPublic  BOOLEAN DEFAULT true,
			CreatedAt TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		ALTER TABLE ChatRoom ADD COLUMN IF NOT EXISTS DeviceId INTEGER;
		CREATE TABLE IF NOT EXISTS ChatMessage (
			MessageId SERIAL PRIMARY KEY,
			RoomId    INTEGER NOT NULL REFERENCES ChatRoom(RoomId) ON DELETE CASCADE,
			SenderId  INTEGER NOT NULL REFERENCES AppUser(UserId) ON DELETE CASCADE,
			Content   TEXT NOT NULL,
			CreatedAt TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_chatmsg_room ON ChatMessage(RoomId, CreatedAt DESC);
		CREATE TABLE IF NOT EXISTS ChatNotification (
			NotifId    SERIAL PRIMARY KEY,
			OwnerId    INTEGER NOT NULL REFERENCES AppUser(UserId) ON DELETE CASCADE,
			RoomId     INTEGER NOT NULL REFERENCES ChatRoom(RoomId) ON DELETE CASCADE,
			DeviceId   INTEGER,
			DeviceName VARCHAR(100),
			SenderName VARCHAR(100),
			Preview    TEXT,
			IsRead     BOOLEAN DEFAULT FALSE,
			CreatedAt  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_chatnotif_owner ON ChatNotification(OwnerId, IsRead, CreatedAt DESC);
		ALTER TABLE ChatMessage ADD COLUMN IF NOT EXISTS ImageUrl TEXT;
		INSERT INTO ChatRoom (RoomName, IsPublic) VALUES ('general', true) ON CONFLICT DO NOTHING;
		INSERT INTO ChatRoom (RoomName, IsPublic) VALUES ('random', true)   ON CONFLICT DO NOTHING;
		INSERT INTO ChatRoom (RoomName, IsPublic) VALUES ('devices', true)  ON CONFLICT DO NOTHING;
	`
	if _, err := db.Exec(chatMigrationSQL); err != nil {
		fmt.Printf("Warning: Chat migration error: %v\n", err)
	} else {
		fmt.Println("\u2713 Chat tables ready")
	}

	// Ensure device Status table has correct device-centric names
	// StatusNo 1-4 = device lifecycle, 5-6 = booking states added by migration 005
	deviceStatusSQL := `
		INSERT INTO Status (StatusNo, Name) VALUES
			(1, 'Available'),
			(2, 'Delivered'),
			(3, 'Returned'),
			(4, 'Overdue'),
			(5, 'Reserved'),
			(6, 'Rented')
		ON CONFLICT (StatusNo) DO UPDATE SET Name = EXCLUDED.Name;
	`
	if _, err := db.Exec(deviceStatusSQL); err != nil {
		fmt.Printf("Warning: device Status upsert error: %v\n", err)
	} else {
		fmt.Println("\u2713 Device Status table ready (Available/Delivered/Returned/Overdue/Reserved/Rented)")
	}

	// Migration 008: Fix RentBill rating triggers to use AvgRating column
	// Owner and Renter tables use AvgRating (added in migration 006) not Rating.
	// The old trigger functions referenced Rating which no longer exists on those tables,
	// causing every INSERT into RentBill to fail with a 500 error.
	fixRatingTriggersSQL := `
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
	`
	if _, err := db.Exec(fixRatingTriggersSQL); err != nil {
		fmt.Printf("Warning: rating trigger fix error: %v\n", err)
	} else {
		fmt.Println("\u2713 RentBill rating triggers fixed (Owner/Renter use AvgRating)")
	}
	// Migration 009: Admin system columns
	adminSchemaMigrationSQL := `
		ALTER TABLE appuser ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT FALSE;
		ALTER TABLE Device  ADD COLUMN IF NOT EXISTS is_admin_device BOOLEAN NOT NULL DEFAULT FALSE;
	`
	if _, err := db.Exec(adminSchemaMigrationSQL); err != nil {
		fmt.Printf("Warning: admin schema migration error: %v\n", err)
	} else {
		fmt.Println("✓ Admin schema columns ready (is_admin, is_admin_device)")
	}

	// Migration 010: Authorized lender column
	lenderSchemaMigrationSQL := `
		ALTER TABLE appuser ADD COLUMN IF NOT EXISTS is_authorized_lender BOOLEAN NOT NULL DEFAULT FALSE;
	`
	if _, err := db.Exec(lenderSchemaMigrationSQL); err != nil {
		fmt.Printf("Warning: authorized_lender migration error: %v\n", err)
	} else {
		fmt.Println("✓ Authorized lender column ready (is_authorized_lender)")
	}

	// Migration 011: Central staff column
	centralStaffMigrationSQL := `
		ALTER TABLE appuser ADD COLUMN IF NOT EXISTS is_central_staff BOOLEAN NOT NULL DEFAULT FALSE;
	`
	if _, err := db.Exec(centralStaffMigrationSQL); err != nil {
		fmt.Printf("Warning: central_staff migration error: %v\n", err)
	} else {
		fmt.Println("✓ Central staff column ready (is_central_staff)")
	}

	// ============================================================
	// Admin accounts seed (สร้าง 3 บัญชี admin ถ้ายังไม่มี)
	// ============================================================
	adminAccounts := []struct{ email, fname, lname string }{
		{"admin@notelet.com", "Admin", "Notelet"},
		{"supervisor@notelet.com", "Supervisor", "Notelet"},
		{"manager@notelet.com", "Manager", "Notelet"},
	}
	adminPassword := "test1234"
	hashedAdminPw, _ := utils.HashPassword(adminPassword)
	for _, acc := range adminAccounts {
		var exists int
		db.QueryRow(`SELECT userid FROM appuser WHERE email = $1`, acc.email).Scan(&exists)
		if exists != 0 {
			// มีอยู่แล้ว — ตรวจสอบให้ is_admin = true
			db.Exec(`UPDATE appuser SET is_admin = true WHERE email = $1`, acc.email)
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			fmt.Printf("Warning: seed admin tx begin: %v\n", err)
			continue
		}
		var userId int
		err = tx.QueryRow(`
			INSERT INTO appuser (email, passwordhash, isactive, is_admin, createdat)
			VALUES ($1, $2, true, true, NOW()) RETURNING userid
		`, acc.email, hashedAdminPw).Scan(&userId)
		if err != nil {
			tx.Rollback()
			fmt.Printf("Warning: seed admin insert appuser (%s): %v\n", acc.email, err)
			continue
		}
		var ownerNo int
		tx.QueryRow(`SELECT COALESCE(MAX(ownerno),0)+1 FROM owner`).Scan(&ownerNo)
		tx.Exec(`INSERT INTO owner (ownerno,name,fname,lname,tel,userid) VALUES ($1,$2,$3,$4,$5,$6)`,
			ownerNo, acc.fname+" "+acc.lname, acc.fname, acc.lname, "", userId)
		var renterNo int
		tx.QueryRow(`SELECT COALESCE(MAX(renterno),0)+1 FROM renter`).Scan(&renterNo)
		tx.Exec(`INSERT INTO renter (renterno,name,fname,lname,tel,userid) VALUES ($1,$2,$3,$4,$5,$6)`,
			renterNo, acc.fname+" "+acc.lname, acc.fname, acc.lname, "", userId)
		if err := tx.Commit(); err != nil {
			fmt.Printf("Warning: seed admin commit (%s): %v\n", acc.email, err)
		} else {
			fmt.Printf("✓ Admin account seeded: %s\n", acc.email)
		}
	}

	// ============================================================
	// Authorized Lender accounts seed (สร้างบัญชีที่มีสิทธิ์เพิ่มอุปกรณ์ยืม)
	// ============================================================
	lenderAccounts := []struct{ email, fname, lname string }{
		{"test675@kmitl.ac.th", "CS", "Department"},
	}
	lenderPassword := "test1234"
	hashedLenderPw, _ := utils.HashPassword(lenderPassword)
	for _, acc := range lenderAccounts {
		var existsId int
		db.QueryRow(`SELECT userid FROM appuser WHERE email = $1`, acc.email).Scan(&existsId)
		if existsId != 0 {
			// มีอยู่แล้ว — ให้ is_authorized_lender = true
			db.Exec(`UPDATE appuser SET is_authorized_lender = true WHERE email = $1`, acc.email)
			fmt.Printf("✓ Authorized lender already exists, updated: %s\n", acc.email)
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			fmt.Printf("Warning: seed lender tx begin: %v\n", err)
			continue
		}
		var userId int
		err = tx.QueryRow(`
			INSERT INTO appuser (email, passwordhash, isactive, is_admin, is_authorized_lender, createdat)
			VALUES ($1, $2, true, false, true, NOW()) RETURNING userid
		`, acc.email, hashedLenderPw).Scan(&userId)
		if err != nil {
			tx.Rollback()
			fmt.Printf("Warning: seed lender insert appuser (%s): %v\n", acc.email, err)
			continue
		}
		var ownerNo int
		tx.QueryRow(`SELECT COALESCE(MAX(ownerno),0)+1 FROM owner`).Scan(&ownerNo)
		tx.Exec(`INSERT INTO owner (ownerno,name,fname,lname,tel,userid) VALUES ($1,$2,$3,$4,$5,$6)`,
			ownerNo, acc.fname+" "+acc.lname, acc.fname, acc.lname, "", userId)
		var renterNo int
		tx.QueryRow(`SELECT COALESCE(MAX(renterno),0)+1 FROM renter`).Scan(&renterNo)
		tx.Exec(`INSERT INTO renter (renterno,name,fname,lname,tel,userid) VALUES ($1,$2,$3,$4,$5,$6)`,
			renterNo, acc.fname+" "+acc.lname, acc.fname, acc.lname, "", userId)
		if err := tx.Commit(); err != nil {
			fmt.Printf("Warning: seed lender commit (%s): %v\n", acc.email, err)
		} else {
			fmt.Printf("✓ Authorized lender account seeded: %s (password: %s)\n", acc.email, lenderPassword)
		}
	}

	// สร้าง controllers
	authController := controllers.NewAuthController(db)
	oauthController := controllers.NewOAuthController(db)
	firebaseController := controllers.NewFirebaseAuthController(db)
	supabaseController := controllers.NewSupabaseAuthController(db)
	deviceController := controllers.NewDeviceController(db)
	uploadController := controllers.NewUploadController("./uploads")
	reviewController := controllers.NewReviewController(db)
	rentalController := controllers.NewRentalController(db)
	chatController := controllers.NewChatController(db)

	// Create a new router
	mux := http.NewServeMux()

	// Setup API routes first
	apiMux := routers.SetupRoutes(authController, oauthController, firebaseController, supabaseController, deviceController, uploadController, reviewController, rentalController, chatController)

	// Mount API routes
	mux.Handle("/api/", apiMux)

	// Health check endpoint (used by Railway)
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Serve uploaded files
	uploadsFS := http.FileServer(http.Dir("./uploads"))
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", uploadsFS))

	// Apply CORS middleware
	handler := routers.ApplyCORS(mux)

	// กำหนด port — Railway inject $PORT, local dev defaults to 3001
	port := ":" + getEnvPort("3001")

	fmt.Printf("✓ Server listening on %s\n", port)
	// Start server
	if err := http.ListenAndServe(port, handler); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}

func getEnvPort(fallback string) string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return fallback
}
