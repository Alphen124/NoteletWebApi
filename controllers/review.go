package controllers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"noteletwebservice-development/middlewares"
)

// ReviewController handles review-related HTTP requests
type ReviewController struct {
	DB *sql.DB
}

// NewReviewController creates a new review controller
func NewReviewController(db *sql.DB) *ReviewController {
	return &ReviewController{DB: db}
}

// GetDeviceReviews handles GET /api/devices/{id}/reviews - Get all reviews for a device (public)
func (rc *ReviewController) GetDeviceReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	// Extract device ID from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/devices/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		respondWithError(w, http.StatusBadRequest, "Device ID is required", "")
		return
	}

	deviceID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid device ID", err.Error())
		return
	}

	rows, err := rc.DB.Query(`
		SELECT 
			rv.ReviewNo,
			rv.DeviceNo,
			rv.ReviewerUserId,
			COALESCE(au.Email, '') as ReviewerEmail,
			rv.Rating,
			COALESCE(rv.Description, '') as Description,
			rv.CreatedAt
		FROM Review rv
		LEFT JOIN AppUser au ON rv.ReviewerUserId = au.UserId
		WHERE rv.DeviceNo = $1
		ORDER BY rv.CreatedAt DESC
	`, deviceID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch reviews", err.Error())
		return
	}
	defer rows.Close()

	type ReviewEntry struct {
		ReviewNo       int       `json:"reviewNo"`
		DeviceNo       int       `json:"deviceNo"`
		ReviewerUserId int       `json:"reviewerUserId"`
		ReviewerEmail  string    `json:"reviewerEmail"`
		Rating         int       `json:"rating"`
		Description    string    `json:"description"`
		CreatedAt      time.Time `json:"createdAt"`
	}

	var reviews []ReviewEntry
	for rows.Next() {
		var entry ReviewEntry
		err := rows.Scan(
			&entry.ReviewNo,
			&entry.DeviceNo,
			&entry.ReviewerUserId,
			&entry.ReviewerEmail,
			&entry.Rating,
			&entry.Description,
			&entry.CreatedAt,
		)
		if err != nil {
			fmt.Printf("Error scanning review row: %v\n", err)
			continue
		}
		reviews = append(reviews, entry)
	}

	if reviews == nil {
		reviews = []ReviewEntry{}
	}

	// Calculate average rating
	avgRating := 0.0
	if len(reviews) > 0 {
		total := 0
		for _, rv := range reviews {
			total += rv.Rating
		}
		avgRating = float64(total) / float64(len(reviews))
	}

	respondWithSuccess(w, http.StatusOK, "Reviews retrieved successfully", map[string]interface{}{
		"reviews":       reviews,
		"totalReviews":  len(reviews),
		"averageRating": avgRating,
	})
}

// CreateDeviceReview handles POST /api/devices/{id}/reviews - Submit a review (protected)
func (rc *ReviewController) CreateDeviceReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	// Get user from context
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	userID := userCtx.UserId

	// Extract device ID from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/devices/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		respondWithError(w, http.StatusBadRequest, "Device ID is required", "")
		return
	}

	deviceID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid device ID", err.Error())
		return
	}

	// Parse request body
	var req struct {
		Rating      int    `json:"rating"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Validate rating
	if req.Rating < 1 || req.Rating > 5 {
		respondWithError(w, http.StatusBadRequest, "Rating must be between 1 and 5", "")
		return
	}

	// Check device exists
	var exists bool
	err = rc.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM Device WHERE DeviceNo = $1)", deviceID).Scan(&exists)
	if err != nil || !exists {
		respondWithError(w, http.StatusNotFound, "Device not found", "")
		return
	}

	// Upsert review (update if already reviewed by this user for this device)
	var reviewNo int
	err = rc.DB.QueryRow(`
		INSERT INTO Review (DeviceNo, ReviewerUserId, Rating, Description, CreatedAt)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT (DeviceNo, ReviewerUserId)
		DO UPDATE SET Rating = EXCLUDED.Rating, Description = EXCLUDED.Description, CreatedAt = CURRENT_TIMESTAMP
		RETURNING ReviewNo
	`, deviceID, userID, req.Rating, req.Description).Scan(&reviewNo)

	if err != nil {
		fmt.Printf("Error creating review: %v\n", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to save review", err.Error())
		return
	}

	respondWithSuccess(w, http.StatusCreated, "Review saved successfully", map[string]interface{}{
		"reviewNo": reviewNo,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// USER-TO-USER REVIEW SYSTEM
// ─────────────────────────────────────────────────────────────────────────────

// CreateUserReview handles POST /api/reviews
// Renter can review Owner after status = "Rental Active" or "Rental Completed"
// Owner can review Renter only after status = "Rental Completed"
func (rc *ReviewController) CreateUserReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	reviewerID := userCtx.UserId

	var req struct {
		RequestNo   int    `json:"requestNo"`
		Rating      int    `json:"rating"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if req.RequestNo == 0 {
		respondWithError(w, http.StatusBadRequest, "requestNo is required", "")
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		respondWithError(w, http.StatusBadRequest, "Rating must be between 1 and 5", "")
		return
	}

	// ── ดึง RentalRequest + owner ──
	var status string
	var renterUserID, ownerUserID int
	err := rc.DB.QueryRow(`
		SELECT rr.Status, rr.RenterUserId, d.UserId
		FROM   RentalRequest rr
		JOIN   Device d ON d.DeviceNo = rr.DeviceNo
		WHERE  rr.RequestNo = $1
	`, req.RequestNo).Scan(&status, &renterUserID, &ownerUserID)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Rental request not found", "")
		return
	}
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}

	// ── ตรวจ role + เงื่อนไข ──
	var reviewerRole string
	var revieweeID int
	switch reviewerID {
	case renterUserID:
		reviewerRole = "renter"
		revieweeID = ownerUserID
		// Renter รีวิวได้หลังจากได้รับอุปกรณ์แล้ว (Rental Active หรือ Rental Completed)
		if status != "Rental Active" && status != "Rental Completed" {
			respondWithError(w, http.StatusForbidden,
				"Renter can only review after the device has been delivered (Rental Active or Rental Completed)", "")
			return
		}
	case ownerUserID:
		reviewerRole = "owner"
		revieweeID = renterUserID
		// Owner รีวิวได้หลังจากได้รับอุปกรณ์คืน (Rental Completed เท่านั้น)
		if status != "Rental Completed" {
			respondWithError(w, http.StatusForbidden,
				"Owner can only review after the device has been returned (Rental Completed)", "")
			return
		}
	default:
		respondWithError(w, http.StatusForbidden, "You are not a participant of this rental", "")
		return
	}

	// ── INSERT (unique constraint บล็อครีวิวซ้ำ) ──
	var reviewNo int
	err = rc.DB.QueryRow(`
		INSERT INTO UserReview
			(RequestNo, ReviewerUserId, RevieweeUserId, ReviewerRole, Rating, Description)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ReviewNo
	`, req.RequestNo, reviewerID, revieweeID, reviewerRole, req.Rating, req.Description).Scan(&reviewNo)
	if err != nil {
		if strings.Contains(err.Error(), "uq_ur_request_role") {
			respondWithError(w, http.StatusConflict, "You have already reviewed this rental", "")
			return
		}
		fmt.Printf("Error creating user review: %v\n", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to save review", err.Error())
		return
	}

	respondWithSuccess(w, http.StatusCreated, "Review submitted successfully", map[string]interface{}{
		"reviewNo": reviewNo,
	})
}

// CheckReviewEligibility handles GET /api/reviews/eligibility?requestNo=42
// Frontend ใช้ disable/enable ปุ่มรีวิว
func (rc *ReviewController) CheckReviewEligibility(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	reviewerID := userCtx.UserId

	requestNoStr := r.URL.Query().Get("requestNo")
	if requestNoStr == "" {
		respondWithError(w, http.StatusBadRequest, "requestNo query parameter is required", "")
		return
	}
	requestNo, err := strconv.Atoi(requestNoStr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid requestNo", err.Error())
		return
	}

	var status string
	var renterUserID, ownerUserID int
	err = rc.DB.QueryRow(`
		SELECT rr.Status, rr.RenterUserId, d.UserId
		FROM   RentalRequest rr
		JOIN   Device d ON d.DeviceNo = rr.DeviceNo
		WHERE  rr.RequestNo = $1
	`, requestNo).Scan(&status, &renterUserID, &ownerUserID)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Rental request not found", "")
		return
	}
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}

	// ระบุ role
	var role string
	switch reviewerID {
	case renterUserID:
		role = "renter"
	case ownerUserID:
		role = "owner"
	default:
		respondWithSuccess(w, http.StatusOK, "Eligibility checked", map[string]interface{}{
			"canReview":       false,
			"alreadyReviewed": false,
			"role":            nil,
			"reason":          "You are not a participant of this rental",
		})
		return
	}

	// ตรวจว่ารีวิวซ้ำหรือยัง
	var alreadyReviewed bool
	rc.DB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM UserReview WHERE RequestNo = $1 AND ReviewerRole = $2)`,
		requestNo, role,
	).Scan(&alreadyReviewed)

	// ตรวจเงื่อนไข status
	canReview := false
	reason := ""
	if alreadyReviewed {
		reason = "You have already reviewed this rental"
	} else if role == "renter" && (status == "Rental Active" || status == "Rental Completed") {
		canReview = true
	} else if role == "owner" && status == "Rental Completed" {
		canReview = true
	} else if role == "renter" {
		reason = "Waiting for device delivery (need Rental Active status)"
	} else {
		reason = "Waiting for device return (need Rental Completed status)"
	}

	respondWithSuccess(w, http.StatusOK, "Eligibility checked", map[string]interface{}{
		"canReview":       canReview,
		"alreadyReviewed": alreadyReviewed,
		"role":            role,
		"currentStatus":   status,
		"reason":          reason,
	})
}

// GetUserReviews handles GET /api/users/{userId}/reviews
// ดูรีวิวทั้งหมดที่ user คนนี้ได้รับ (public)
// Optional query: ?role=owner|renter
func (rc *ReviewController) GetUserReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	// Extract userId from /api/users/{userId}/reviews
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/users/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		respondWithError(w, http.StatusBadRequest, "User ID is required", "")
		return
	}
	userID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid user ID", err.Error())
		return
	}

	roleFilter := r.URL.Query().Get("role")

	query := `
		SELECT
			ur.ReviewNo,
			ur.RequestNo,
			ur.ReviewerUserId,
			COALESCE(au.Email, '')     AS ReviewerEmail,
			ur.ReviewerRole,
			ur.Rating,
			COALESCE(ur.Description, '') AS Description,
			COALESCE(ur.ReplyText, '')   AS ReplyText,
			ur.RepliedAt,
			ur.CreatedAt
		FROM UserReview ur
		LEFT JOIN AppUser au ON au.UserId = ur.ReviewerUserId
		WHERE ur.RevieweeUserId = $1
	`
	args := []interface{}{userID}
	if roleFilter == "owner" || roleFilter == "renter" {
		query += " AND ur.ReviewerRole = $2"
		args = append(args, roleFilter)
	}
	query += " ORDER BY ur.CreatedAt DESC"

	rows, err := rc.DB.Query(query, args...)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch reviews", err.Error())
		return
	}
	defer rows.Close()

	type UserReviewEntry struct {
		ReviewNo       int        `json:"reviewNo"`
		RequestNo      int        `json:"requestNo"`
		ReviewerUserId int        `json:"reviewerUserId"`
		ReviewerEmail  string     `json:"reviewerEmail"`
		ReviewerRole   string     `json:"reviewerRole"`
		Rating         int        `json:"rating"`
		Description    string     `json:"description"`
		ReplyText      string     `json:"replyText"`
		RepliedAt      *time.Time `json:"repliedAt"`
		CreatedAt      time.Time  `json:"createdAt"`
	}

	var reviews []UserReviewEntry
	for rows.Next() {
		var entry UserReviewEntry
		if err := rows.Scan(
			&entry.ReviewNo, &entry.RequestNo,
			&entry.ReviewerUserId, &entry.ReviewerEmail, &entry.ReviewerRole,
			&entry.Rating, &entry.Description,
			&entry.ReplyText, &entry.RepliedAt, &entry.CreatedAt,
		); err != nil {
			fmt.Printf("Error scanning user review row: %v\n", err)
			continue
		}
		reviews = append(reviews, entry)
	}
	if reviews == nil {
		reviews = []UserReviewEntry{}
	}

	// คำนวณ avg rating
	avgAsOwner, avgAsRenter := 0.0, 0.0
	countOwner, countRenter := 0, 0
	for _, rv := range reviews {
		if rv.ReviewerRole == "renter" { // renter reviewed → owner avg
			avgAsOwner += float64(rv.Rating)
			countOwner++
		} else {
			avgAsRenter += float64(rv.Rating)
			countRenter++
		}
	}
	if countOwner > 0 {
		avgAsOwner = avgAsOwner / float64(countOwner)
	}
	if countRenter > 0 {
		avgAsRenter = avgAsRenter / float64(countRenter)
	}

	respondWithSuccess(w, http.StatusOK, "Reviews retrieved successfully", map[string]interface{}{
		"reviews":           reviews,
		"totalReviews":      len(reviews),
		"avgRatingAsOwner":  avgAsOwner,
		"avgRatingAsRenter": avgAsRenter,
	})
}

// GetUserRating handles GET /api/users/{userId}/rating
// ดึงค่าเฉลี่ย rating ของ user ทั้งในฐานะ Owner และ Renter (public)
func (rc *ReviewController) GetUserRating(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/users/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		respondWithError(w, http.StatusBadRequest, "User ID is required", "")
		return
	}
	userID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid user ID", err.Error())
		return
	}

	type RatingSummary struct {
		AvgRating    float64 `json:"avgRating"`
		TotalReviews int     `json:"totalReviews"`
	}

	queryRating := func(role string) RatingSummary {
		var avg sql.NullFloat64
		var count int
		rc.DB.QueryRow(`
			SELECT ROUND(AVG(Rating)::NUMERIC, 2), COUNT(*)
			FROM UserReview
			WHERE RevieweeUserId = $1 AND ReviewerRole = $2
		`, userID, role).Scan(&avg, &count)
		s := RatingSummary{TotalReviews: count}
		if avg.Valid {
			s.AvgRating = avg.Float64
		}
		return s
	}

	respondWithSuccess(w, http.StatusOK, "Rating retrieved successfully", map[string]interface{}{
		"userId":   userID,
		"asOwner":  queryRating("renter"), // renter rated owner
		"asRenter": queryRating("owner"),  // owner rated renter
	})
}

// ReplyToReview handles PATCH /api/reviews/{reviewNo}/reply
// Reviewee (ผู้ถูกรีวิว) ตอบกลับได้ครั้งเดียว
func (rc *ReviewController) ReplyToReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	currentUserID := userCtx.UserId

	// Extract reviewNo from /api/reviews/{reviewNo}/reply
	path := strings.TrimPrefix(r.URL.Path, "/api/reviews/")
	path = strings.TrimSuffix(path, "/reply")
	reviewNo, err := strconv.Atoi(path)
	if err != nil || reviewNo == 0 {
		respondWithError(w, http.StatusBadRequest, "Invalid review ID", "")
		return
	}

	var req struct {
		ReplyText string `json:"replyText"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if strings.TrimSpace(req.ReplyText) == "" {
		respondWithError(w, http.StatusBadRequest, "replyText is required", "")
		return
	}

	// ตรวจว่า currentUser เป็น reviewee ของรีวิวนี้
	var revieweeID int
	var existingReply sql.NullString
	err = rc.DB.QueryRow(
		`SELECT RevieweeUserId, ReplyText FROM UserReview WHERE ReviewNo = $1`, reviewNo,
	).Scan(&revieweeID, &existingReply)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Review not found", "")
		return
	}
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}
	if revieweeID != currentUserID {
		respondWithError(w, http.StatusForbidden, "Only the reviewee can reply to this review", "")
		return
	}
	if existingReply.Valid && existingReply.String != "" {
		respondWithError(w, http.StatusConflict, "You have already replied to this review", "")
		return
	}

	_, err = rc.DB.Exec(
		`UPDATE UserReview SET ReplyText = $1, RepliedAt = CURRENT_TIMESTAMP WHERE ReviewNo = $2`,
		req.ReplyText, reviewNo,
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to save reply", err.Error())
		return
	}

	respondWithSuccess(w, http.StatusOK, "Reply saved successfully", nil)
}

// GetRenterProfile handles GET /api/users/{userId}/renter-profile
// ดึงข้อมูลโปรไฟล์ผู้เช่า พร้อม stats และรีวิวที่ได้รับในฐานะผู้เช่า (public)
func (rc *ReviewController) GetRenterProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	// Extract userId from /api/users/{userId}/renter-profile
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/users/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		respondWithError(w, http.StatusBadRequest, "User ID is required", "")
		return
	}
	userID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid user ID", err.Error())
		return
	}

	// ── 1. ดึงข้อมูลผู้ใช้จาก AppUser + Renter ──
	var email string
	var fname, lname, tel sql.NullString
	err = rc.DB.QueryRow(`
		SELECT au.Email,
		       COALESCE(rt.FName, ''),
		       COALESCE(rt.LName, ''),
		       COALESCE(rt.Tel, '')
		FROM AppUser au
		LEFT JOIN Renter rt ON rt.UserId = au.UserId
		WHERE au.UserId = $1
	`, userID).Scan(&email, &fname, &lname, &tel)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "User not found", "")
		return
	}
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}

	// ── 2. Stats: totalCompletedRentals ──
	var totalCompleted int
	rc.DB.QueryRow(`
		SELECT COUNT(*) FROM RentalRequest
		WHERE RenterUserId = $1 AND Status = 'Rental Completed'
	`, userID).Scan(&totalCompleted)

	// ── 3. Stats: averageRating + reviewCount (owner reviewed renter) ──
	var avgRating sql.NullFloat64
	var reviewCount int
	rc.DB.QueryRow(`
		SELECT ROUND(AVG(Rating)::NUMERIC, 2), COUNT(*)
		FROM UserReview
		WHERE RevieweeUserId = $1 AND ReviewerRole = 'owner'
	`, userID).Scan(&avgRating, &reviewCount)

	avgRatingVal := 0.0
	if avgRating.Valid {
		avgRatingVal = avgRating.Float64
	}

	// ── 4. Reviews list ──
	rows, err := rc.DB.Query(`
		SELECT
			ur.ReviewNo,
			ur.Rating,
			COALESCE(ur.Description, '')                               AS ReviewText,
			COALESCE(d.DeviceName, '')                                 AS DeviceName,
			COALESCE(
				NULLIF(TRIM(COALESCE(ow.FName,'') || ' ' || COALESCE(ow.LName,'')), ''),
				au_r.Email,
				''
			)                                                          AS ReviewerName,
			ur.CreatedAt
		FROM UserReview ur
		LEFT JOIN RentalRequest rr ON rr.RequestNo = ur.RequestNo
		LEFT JOIN Device d         ON d.DeviceNo = rr.DeviceNo
		LEFT JOIN AppUser au_r     ON au_r.UserId = ur.ReviewerUserId
		LEFT JOIN Owner ow         ON ow.UserId = ur.ReviewerUserId
		WHERE ur.RevieweeUserId = $1
		  AND ur.ReviewerRole = 'owner'
		ORDER BY ur.CreatedAt DESC
	`, userID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch reviews", err.Error())
		return
	}
	defer rows.Close()

	type ReviewItem struct {
		ReviewID     int       `json:"reviewId"`
		Rating       int       `json:"rating"`
		ReviewText   string    `json:"reviewText"`
		DeviceName   string    `json:"deviceName"`
		ReviewerName string    `json:"reviewerName"`
		CreatedAt    time.Time `json:"createdAt"`
	}

	var reviews []ReviewItem
	for rows.Next() {
		var item ReviewItem
		if err := rows.Scan(
			&item.ReviewID, &item.Rating, &item.ReviewText,
			&item.DeviceName, &item.ReviewerName, &item.CreatedAt,
		); err != nil {
			fmt.Printf("Error scanning renter profile review row: %v\n", err)
			continue
		}
		reviews = append(reviews, item)
	}
	if reviews == nil {
		reviews = []ReviewItem{}
	}

	respondWithSuccess(w, http.StatusOK, "Renter profile retrieved successfully", map[string]interface{}{
		"userId": userID,
		"fname":  fname.String,
		"lname":  lname.String,
		"email":  email,
		"tel":    tel.String,
		"stats": map[string]interface{}{
			"totalCompletedRentals": totalCompleted,
			"averageRating":         avgRatingVal,
			"reviewCount":           reviewCount,
		},
		"reviews": reviews,
	})
}
