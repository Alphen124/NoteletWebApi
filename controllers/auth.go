package controllers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"noteletwebservice-development/middlewares"
	"noteletwebservice-development/models"
	"noteletwebservice-development/services/jwt"
	"noteletwebservice-development/types/requests"
	"noteletwebservice-development/types/responses"
	"noteletwebservice-development/utils"
)

// adminEmailWhitelist รายชื่ออีเมลที่ได้สิทธิ์ admin อัตโนมัติ
var adminEmailWhitelist = []string{
	"admin@notelet.com",
	"supervisor@notelet.com",
	"manager@notelet.com",
}

func isAdminEmail(email string) bool {
	e := strings.ToLower(strings.TrimSpace(email))
	for _, a := range adminEmailWhitelist {
		if a == e {
			return true
		}
	}
	return false
}

type AuthController struct {
	DB *sql.DB
}

// NewAuthController สร้าง instance ของ AuthController
func NewAuthController(db *sql.DB) *AuthController {
	return &AuthController{DB: db}
}

// Register สำหรับลงทะเบียนผู้ใช้ใหม่
func (ac *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	var req requests.RegisterRequest

	// Decode JSON request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Debugging log to inspect the received payload
	log.Printf("Received registration request: %+v", req)
	log.Printf("FName: '%s', LName: '%s', Tel: '%s'", req.FName, req.LName, req.Tel)

	// Validate required fields
	if req.FName == "" || req.LName == "" || req.Tel == "" {
		respondWithError(w, http.StatusBadRequest, "All fields are required", "")
		return
	}

	// ตรวจสอบว่าเป็นอีเมล @kmitl.ac.th หรืออยู่ใน whitelist
	if !isAdminEmail(req.Email) && !utils.IsKMITLEmail(req.Email) {
		respondWithError(w, http.StatusBadRequest, "Only @kmitl.ac.th email addresses are allowed", "")
		return
	}

	// ตรวจสอบว่าอีเมลมีอยู่ในระบบแล้วหรือไม่
	var existingUserId int
	err := ac.DB.QueryRow("SELECT userid FROM appuser WHERE email = $1", strings.ToLower(req.Email)).Scan(&existingUserId)
	if err != sql.ErrNoRows {
		respondWithError(w, http.StatusConflict, "Email already registered", "")
		return
	}

	// Hash password
	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to process password", err.Error())
		return
	}

	// เริ่ม transaction
	tx, err := ac.DB.Begin()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}
	defer tx.Rollback()

	// สร้างผู้ใช้ใน AppUser table (ตั้ง is_admin=true สำหรับ whitelist emails)
	var userId int
	err = tx.QueryRow(`
		INSERT INTO appuser (email, passwordhash, isactive, is_admin, createdat)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING userid
	`, strings.ToLower(req.Email), hashedPassword, true, isAdminEmail(req.Email)).Scan(&userId)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create user", err.Error())
		return
	}

	// สร้างทั้ง Owner และ Renter พร้อมกัน
	// สร้าง Owner
	var nextOwnerNo int
	err = tx.QueryRow(`SELECT COALESCE(MAX(ownerno), 0) + 1 FROM owner`).Scan(&nextOwnerNo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to generate owner number", err.Error())
		return
	}

	_, err = tx.Exec(`
		INSERT INTO owner (ownerno, name, fname, lname, tel, userid)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, nextOwnerNo, req.FName+" "+req.LName, req.FName, req.LName, req.Tel, userId)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create owner profile", err.Error())
		return
	}
	log.Printf("Created Owner: OwnerNo=%d, Name='%s', FName='%s', LName='%s', Tel='%s', UserId=%d",
		nextOwnerNo, req.FName+" "+req.LName, req.FName, req.LName, req.Tel, userId)

	// สร้าง Renter
	var nextRenterNo int
	err = tx.QueryRow(`SELECT COALESCE(MAX(renterno), 0) + 1 FROM renter`).Scan(&nextRenterNo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to generate renter number", err.Error())
		return
	}

	_, err = tx.Exec(`
		INSERT INTO renter (renterno, name, fname, lname, tel, userid)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, nextRenterNo, req.FName+" "+req.LName, req.FName, req.LName, req.Tel, userId)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create renter profile", err.Error())
		return
	}
	log.Printf("Created Renter: RenterNo=%d, Name='%s', FName='%s', LName='%s', Tel='%s', UserId=%d",
		nextRenterNo, req.FName+" "+req.LName, req.FName, req.LName, req.Tel, userId)

	// Commit transaction
	if err = tx.Commit(); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to complete registration", err.Error())
		return
	}

	// สร้าง JWT tokens (is_admin=false สำหรับผู้ใช้ทั่วไป)
	accessToken, refreshToken, err := jwt.GenerateTokenPair(userId, req.Email, false)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to generate tokens", err.Error())
		return
	}

	// ส่ง response
	respondWithSuccess(w, http.StatusCreated, "Registration successful", responses.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: responses.UserResponse{
			UserId:   userId,
			Email:    req.Email,
			IsActive: true,
			FName:    req.FName,
			LName:    req.LName,
			Tel:      req.Tel,
		},
	})
}

// Login สำหรับเข้าสู่ระบบ
func (ac *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	var req requests.LoginRequest

	// Decode JSON request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// ดึงข้อมูลผู้ใช้จากฐานข้อมูล (รวม is_admin)
	var user models.AppUser
	err := ac.DB.QueryRow(`
		SELECT userid, email, passwordhash, isactive, COALESCE(is_admin, false), createdat
		FROM appuser
		WHERE email = $1
	`, strings.ToLower(req.Email)).Scan(
		&user.UserId, &user.Email, &user.PasswordHash,
		&user.IsActive, &user.IsAdmin, &user.CreatedAt,
	)

	// อีเมลใน whitelist ได้สิทธิ์ admin อัตโนมัติ
	if isAdminEmail(req.Email) {
		user.IsAdmin = true
		// อัปเดต DB ให้ตรงกัน (idempotent)
		ac.DB.Exec(`UPDATE appuser SET is_admin = true WHERE email = $1`, strings.ToLower(req.Email))
	}

	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusUnauthorized, "Invalid email or password", "")
		return
	}

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}

	// ตรวจสอบว่าเป็นอีเมล @kmitl.ac.th (ยกเว้น admin)
	if !user.IsAdmin && !utils.IsKMITLEmail(req.Email) {
		respondWithError(w, http.StatusUnauthorized, "Only @kmitl.ac.th email addresses are allowed", "")
		return
	}

	// ตรวจสอบว่า account active หรือไม่
	if !user.IsActive {
		respondWithError(w, http.StatusUnauthorized, "Account is inactive", "")
		return
	}

	// ตรวจสอบ password
	if !utils.CheckPasswordHash(req.Password, user.PasswordHash) {
		respondWithError(w, http.StatusUnauthorized, "Invalid email or password", "")
		return
	}

	// ดึงข้อมูลทั้ง Owner และ Renter
	var ownerNo, renterNo sql.NullInt64
	var fname, lname, tel string
	var ownerRating, renterRating sql.NullFloat64

	// ดึงข้อมูล Owner
	ac.DB.QueryRow(`
		SELECT ownerno, fname, lname, tel, avgrating FROM owner WHERE userid = $1
	`, user.UserId).Scan(&ownerNo, &fname, &lname, &tel, &ownerRating)

	// ดึงข้อมูล Renter
	ac.DB.QueryRow(`
		SELECT renterno, avgrating FROM renter WHERE userid = $1
	`, user.UserId).Scan(&renterNo, &renterRating)

	// สร้าง JWT tokens (รวม is_admin)
	accessToken, refreshToken, err := jwt.GenerateTokenPair(user.UserId, user.Email, user.IsAdmin)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to generate tokens", err.Error())
		return
	}

	// ส่ง response พร้อมข้อมูลทั้ง Owner และ Renter
	responseData := responses.DualRoleUserResponse{
		UserId:   user.UserId,
		Email:    user.Email,
		IsActive: user.IsActive,
		IsAdmin:  user.IsAdmin,
		FName:    fname,
		LName:    lname,
		Tel:      tel,
	}

	if ownerNo.Valid {
		responseData.OwnerNo = int(ownerNo.Int64)
		if ownerRating.Valid {
			responseData.OwnerRating = int(ownerRating.Float64)
		}
	}

	if renterNo.Valid {
		responseData.RenterNo = int(renterNo.Int64)
		if renterRating.Valid {
			responseData.RenterRating = int(renterRating.Float64)
		}
	}

	// ส่ง response
	respondWithSuccess(w, http.StatusOK, "Login successful", responses.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         responseData,
	})
}

// RefreshToken สำหรับขอ access token ใหม่
func (ac *AuthController) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req requests.RefreshTokenRequest

	// Decode JSON request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Validate refresh token
	claims, err := jwt.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid or expired refresh token", err.Error())
		return
	}

	// ตรวจสอบว่าผู้ใช้ยังคง active อยู่หรือไม่
	var isActive bool
	err = ac.DB.QueryRow(`
		SELECT isactive FROM appuser WHERE userid = $1
	`, claims.UserId).Scan(&isActive)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}

	if !isActive {
		respondWithError(w, http.StatusUnauthorized, "Account is inactive", "")
		return
	}

	// ดึง is_admin ของผู้ใช้จาก DB
	var isAdmin bool
	ac.DB.QueryRow(`SELECT COALESCE(is_admin, false) FROM appuser WHERE userid = $1`, claims.UserId).Scan(&isAdmin)

	// สร้าง access token ใหม่ (รวม is_admin)
	accessToken, err := jwt.GenerateAccessToken(claims.UserId, claims.Email, isAdmin)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to generate access token", err.Error())
		return
	}

	// ส่ง response
	respondWithSuccess(w, http.StatusOK, "Token refreshed successfully", map[string]string{
		"access_token": accessToken,
	})
}

// GetProfile ดึงข้อมูลโปรไฟล์ของผู้ใช้ (ต้องใช้ร่วมกับ AuthMiddleware)
func (ac *AuthController) GetProfile(w http.ResponseWriter, r *http.Request) {
	// Import middlewares package
	userCtx, ok := middlewares.GetUserFromContext(r)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "User context not found", "")
		return
	}

	// ดึงข้อมูลผู้ใช้จากฐานข้อมูล
	var user models.AppUser
	err := ac.DB.QueryRow(`
		SELECT userid, email, isactive, COALESCE(is_admin, false), createdat
		FROM appuser
		WHERE userid = $1
	`, userCtx.UserId).Scan(
		&user.UserId, &user.Email,
		&user.IsActive, &user.IsAdmin, &user.CreatedAt,
	)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}

	// ดึงข้อมูลทั้ง Owner และ Renter
	var ownerNo, renterNo sql.NullInt64
	var fname, lname, tel string
	var ownerRating, renterRating sql.NullFloat64

	// ดึงข้อมูล Owner
	ac.DB.QueryRow(`
		SELECT ownerno, fname, lname, tel, avgrating FROM owner WHERE userid = $1
	`, user.UserId).Scan(&ownerNo, &fname, &lname, &tel, &ownerRating)

	// ดึงข้อมูล Renter
	ac.DB.QueryRow(`
		SELECT renterno, avgrating FROM renter WHERE userid = $1
	`, user.UserId).Scan(&renterNo, &renterRating)

	// ส่ง response พร้อมข้อมูลทั้ง Owner และ Renter
	responseData := responses.DualRoleUserResponse{
		UserId:   user.UserId,
		Email:    user.Email,
		IsActive: user.IsActive,
		IsAdmin:  user.IsAdmin,
		FName:    fname,
		LName:    lname,
		Tel:      tel,
	}

	if ownerNo.Valid {
		responseData.OwnerNo = int(ownerNo.Int64)
		if ownerRating.Valid {
			responseData.OwnerRating = int(ownerRating.Float64)
		}
	}

	if renterNo.Valid {
		responseData.RenterNo = int(renterNo.Int64)
		if renterRating.Valid {
			responseData.RenterRating = int(renterRating.Float64)
		}
	}

	// ส่ง response
	respondWithSuccess(w, http.StatusOK, "Profile retrieved successfully", responseData)
}

// AdminRegister สร้างบัญชี admin (ใช้ได้กับอีเมลใดก็ได้ ต้องใช้ X-Admin-Secret header)
func (ac *AuthController) AdminRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	// ตรวจสอบ admin secret header
	adminSecret := os.Getenv("ADMIN_SECRET")
	if adminSecret == "" {
		adminSecret = "notelet-admin-secret-2026"
	}
	if r.Header.Get("X-Admin-Secret") != adminSecret {
		respondWithError(w, http.StatusForbidden, "Invalid admin secret", "")
		return
	}

	var req requests.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Email == "" || req.Password == "" || req.FName == "" || req.LName == "" {
		respondWithError(w, http.StatusBadRequest, "Email, password, fname, lname are required", "")
		return
	}

	// ตรวจ email ซ้ำ
	var existing int
	err := ac.DB.QueryRow("SELECT userid FROM appuser WHERE email = $1", strings.ToLower(req.Email)).Scan(&existing)
	if err != sql.ErrNoRows {
		respondWithError(w, http.StatusConflict, "Email already registered", "")
		return
	}

	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to process password", err.Error())
		return
	}

	tx, err := ac.DB.Begin()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}
	defer tx.Rollback()

	var userId int
	err = tx.QueryRow(`
		INSERT INTO appuser (email, passwordhash, isactive, is_admin, createdat)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING userid
	`, strings.ToLower(req.Email), hashedPassword, true, true).Scan(&userId)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create admin user", err.Error())
		return
	}

	// สร้าง Owner record สำหรับ admin ด้วย
	var nextOwnerNo int
	tx.QueryRow(`SELECT COALESCE(MAX(ownerno), 0) + 1 FROM owner`).Scan(&nextOwnerNo)
	tx.Exec(`INSERT INTO owner (ownerno, name, fname, lname, tel, userid) VALUES ($1, $2, $3, $4, $5, $6)`,
		nextOwnerNo, req.FName+" "+req.LName, req.FName, req.LName,
		nullableTel(req.Tel), userId)

	// สร้าง Renter record สำหรับ admin ด้วย
	var nextRenterNo int
	tx.QueryRow(`SELECT COALESCE(MAX(renterno), 0) + 1 FROM renter`).Scan(&nextRenterNo)
	tx.Exec(`INSERT INTO renter (renterno, name, fname, lname, tel, userid) VALUES ($1, $2, $3, $4, $5, $6)`,
		nextRenterNo, req.FName+" "+req.LName, req.FName, req.LName,
		nullableTel(req.Tel), userId)

	if err = tx.Commit(); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to complete registration", err.Error())
		return
	}

	accessToken, refreshToken, err := jwt.GenerateTokenPair(userId, req.Email, true)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to generate tokens", err.Error())
		return
	}

	respondWithSuccess(w, http.StatusCreated, "Admin registered successfully", responses.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: responses.DualRoleUserResponse{
			UserId:   userId,
			Email:    req.Email,
			IsActive: true,
			IsAdmin:  true,
			FName:    req.FName,
			LName:    req.LName,
			Tel:      req.Tel,
		},
	})
}

func nullableTel(tel string) string {
	if tel == "" {
		return "N/A"
	}
	return tel
}
func respondWithError(w http.ResponseWriter, code int, message, error string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(responses.ErrorResponse{
		Success: false,
		Message: message,
		Error:   error,
	})
}

func respondWithSuccess(w http.ResponseWriter, code int, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(responses.SuccessResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}
