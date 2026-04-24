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
			rv.RatingCondition,
			rv.RatingValue,
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
		ReviewNo        int       `json:"reviewNo"`
		DeviceNo        int       `json:"deviceNo"`
		ReviewerUserId  int       `json:"reviewerUserId"`
		ReviewerEmail   string    `json:"reviewerEmail"`
		Rating          int       `json:"rating"`
		Description     string    `json:"description"`
		RatingCondition *int      `json:"ratingCondition"`
		RatingValue     *int      `json:"ratingValue"`
		CreatedAt       time.Time `json:"createdAt"`
		RatingCommunication int `json:"ratingCommunication"`
		RatingPunctuality   int `json:"ratingPunctuality"`
		RatingAccuracy      int `json:"ratingAccuracy"`
		RatingCare          int `json:"ratingCare"`
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
			&entry.RatingCondition,
			&entry.RatingValue,
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
		Rating          int    `json:"rating"`
		Description     string `json:"description"`
		RatingCondition *int   `json:"ratingCondition"`
		RatingValue     *int   `json:"ratingValue"`
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
		INSERT INTO Review (DeviceNo, ReviewerUserId, Rating, Description, RatingCondition, RatingValue, CreatedAt)
		VALUES ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
		ON CONFLICT (DeviceNo, ReviewerUserId)
		DO UPDATE SET Rating = EXCLUDED.Rating, Description = EXCLUDED.Description,
		             RatingCondition = EXCLUDED.RatingCondition, RatingValue = EXCLUDED.RatingValue,
		             CreatedAt = CURRENT_TIMESTAMP
		RETURNING ReviewNo
	`, deviceID, userID, req.Rating, req.Description, req.RatingCondition, req.RatingValue).Scan(&reviewNo)

	if err != nil {
		fmt.Printf("Error creating review: %v\n", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to save review", err.Error())
		return
	}

	respondWithSuccess(w, http.StatusCreated, "Review saved successfully", map[string]interface{}{
		"reviewNo": reviewNo,
	})
}

// เนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌ
// USER-TO-USER REVIEW SYSTEM
// เนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌเนโ€โฌ

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
		RequestNo           int    `json:"requestNo"`
		Rating              int    `json:"rating"`
		Description         string `json:"description"`
		RatingCommunication *int   `json:"ratingCommunication"`
		RatingPunctuality   *int   `json:"ratingPunctuality"`
		RatingAccuracy      *int   `json:"ratingAccuracy"`
		RatingCare          *int   `json:"ratingCare"`
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

	// เนโ€โฌเนโ€โฌ เน€เธโ€เน€เธเธ–เน€เธย RentalRequest + owner เนโ€โฌเนโ€โฌ
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

	// เนโ€โฌเนโ€โฌ เน€เธโ€ขเน€เธเธเน€เธเธเน€เธย role + เน€เธโฌเน€เธยเน€เธเธ—เน€เธยเน€เธเธเน€เธยเน€เธยเน€เธย เนโ€โฌเนโ€โฌ
	var reviewerRole string
	var revieweeID int
	switch reviewerID {
	case renterUserID:
		reviewerRole = "renter"
		revieweeID = ownerUserID
		// Renter เน€เธเธเน€เธเธ•เน€เธเธเน€เธเธ”เน€เธเธเน€เธยเน€เธโ€เน€เธยเน€เธเธเน€เธเธ…เน€เธเธ‘เน€เธยเน€เธยเน€เธเธ’เน€เธยเน€เธยเน€เธโ€เน€เธยเน€เธเธเน€เธเธ‘เน€เธยเน€เธเธเน€เธเธเน€เธยเน€เธยเน€เธเธเน€เธโ€เน€เธยเน€เธยเน€เธเธ…เน€เธยเน€เธเธ (Rental Active เน€เธเธเน€เธเธเน€เธเธ—เน€เธเธ Rental Completed)
		if status != "Rental Active" && status != "Rental Completed" {
			respondWithError(w, http.StatusForbidden,
				"Renter can only review after the device has been delivered (Rental Active or Rental Completed)", "")
			return
		}
	case ownerUserID:
		reviewerRole = "owner"
		revieweeID = renterUserID
		// Owner เน€เธเธเน€เธเธ•เน€เธเธเน€เธเธ”เน€เธเธเน€เธยเน€เธโ€เน€เธยเน€เธเธเน€เธเธ…เน€เธเธ‘เน€เธยเน€เธยเน€เธเธ’เน€เธยเน€เธยเน€เธโ€เน€เธยเน€เธเธเน€เธเธ‘เน€เธยเน€เธเธเน€เธเธเน€เธยเน€เธยเน€เธเธเน€เธโ€เน€เธยเน€เธยเน€เธเธ—เน€เธย (Rental Completed เน€เธโฌเน€เธโ€”เน€เธยเน€เธเธ’เน€เธยเน€เธเธ‘เน€เธยเน€เธย)
		if status != "Rental Completed" {
			respondWithError(w, http.StatusForbidden,
				"Owner can only review after the device has been returned (Rental Completed)", "")
			return
		}
	default:
		respondWithError(w, http.StatusForbidden, "You are not a participant of this rental", "")
		return
	}

	// เนโ€โฌเนโ€โฌ INSERT (unique constraint เน€เธยเน€เธเธ…เน€เธยเน€เธเธเน€เธยเน€เธเธเน€เธเธ•เน€เธเธเน€เธเธ”เน€เธเธเน€เธยเน€เธยเน€เธเธ“) เนโ€โฌเนโ€โฌ
	var reviewNo int
	err = rc.DB.QueryRow(`
		INSERT INTO UserReview
			(RequestNo, ReviewerUserId, RevieweeUserId, ReviewerRole, Rating, Description,
			 RatingCommunication, RatingPunctuality, RatingAccuracy, RatingCare)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING ReviewNo
	`, req.RequestNo, reviewerID, revieweeID, reviewerRole, req.Rating, req.Description,
		req.RatingCommunication, req.RatingPunctuality, req.RatingAccuracy, req.RatingCare).Scan(&reviewNo)
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
// Frontend เน€เธยเน€เธยเน€เธย disable/enable เน€เธยเน€เธเธเน€เธยเน€เธเธเน€เธเธเน€เธเธ•เน€เธเธเน€เธเธ”เน€เธเธ
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

	// เน€เธเธเน€เธเธเน€เธยเน€เธเธ role
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

	// เน€เธโ€ขเน€เธเธเน€เธเธเน€เธยเน€เธเธเน€เธยเน€เธเธ’เน€เธเธเน€เธเธ•เน€เธเธเน€เธเธ”เน€เธเธเน€เธยเน€เธยเน€เธเธ“เน€เธเธเน€เธเธเน€เธเธ—เน€เธเธเน€เธเธเน€เธเธ‘เน€เธย
	var alreadyReviewed bool
	rc.DB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM UserReview WHERE RequestNo = $1 AND ReviewerRole = $2)`,
		requestNo, role,
	).Scan(&alreadyReviewed)

	// เน€เธโ€ขเน€เธเธเน€เธเธเน€เธยเน€เธโฌเน€เธยเน€เธเธ—เน€เธยเน€เธเธเน€เธยเน€เธยเน€เธย status
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
// เน€เธโ€เน€เธเธเน€เธเธเน€เธเธ•เน€เธเธเน€เธเธ”เน€เธเธเน€เธโ€”เน€เธเธ‘เน€เธยเน€เธยเน€เธเธเน€เธเธเน€เธโ€เน€เธโ€”เน€เธเธ•เน€เธย user เน€เธยเน€เธยเน€เธยเน€เธเธ•เน€เธยเน€เธยเน€เธโ€เน€เธยเน€เธเธเน€เธเธ‘เน€เธย (public)
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
			COALESCE(au.Email, '')        AS ReviewerEmail,
			ur.ReviewerRole,
			ur.Rating,
			COALESCE(ur.Description, '')  AS Description,
			COALESCE(ur.ReplyText, '')    AS ReplyText,
			ur.RepliedAt,
			ur.RatingCommunication,
			ur.RatingPunctuality,
			ur.RatingAccuracy,
			ur.RatingCare,
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
		ReviewNo            int        `json:"reviewNo"`
		RequestNo           int        `json:"requestNo"`
		ReviewerUserId      int        `json:"reviewerUserId"`
		ReviewerEmail       string     `json:"reviewerEmail"`
		ReviewerRole        string     `json:"reviewerRole"`
		Rating              int        `json:"rating"`
		Description         string     `json:"description"`
		ReplyText           string     `json:"replyText"`
		RepliedAt           *time.Time `json:"repliedAt"`
		RatingCommunication *int       `json:"ratingCommunication"`
		RatingPunctuality   *int       `json:"ratingPunctuality"`
		RatingAccuracy      *int       `json:"ratingAccuracy"`
		RatingCare          *int       `json:"ratingCare"`
		CreatedAt           time.Time  `json:"createdAt"`
	}

	var reviews []UserReviewEntry
	for rows.Next() {
		var entry UserReviewEntry
		if err := rows.Scan(
			&entry.ReviewNo, &entry.RequestNo,
			&entry.ReviewerUserId, &entry.ReviewerEmail, &entry.ReviewerRole,
			&entry.Rating, &entry.Description,
			&entry.ReplyText, &entry.RepliedAt,
			&entry.RatingCommunication, &entry.RatingPunctuality,
			&entry.RatingAccuracy, &entry.RatingCare,
			&entry.CreatedAt,
		); err != nil {
			fmt.Printf("Error scanning user review row: %v\n", err)
			continue
		}
		reviews = append(reviews, entry)
	}
	if reviews == nil {
		reviews = []UserReviewEntry{}
	}

	// เน€เธยเน€เธเธ“เน€เธยเน€เธเธเน€เธโ€ avg rating
	avgAsOwner, avgAsRenter := 0.0, 0.0
	countOwner, countRenter := 0, 0
	for _, rv := range reviews {
		if rv.ReviewerRole == "renter" { // renter reviewed เนยโ€ owner avg
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
// เน€เธโ€เน€เธเธ–เน€เธยเน€เธยเน€เธยเน€เธเธ’เน€เธโฌเน€เธยเน€เธเธ…เน€เธเธ•เน€เธยเน€เธเธ rating เน€เธยเน€เธเธเน€เธย user เน€เธโ€”เน€เธเธ‘เน€เธยเน€เธยเน€เธยเน€เธยเน€เธยเน€เธเธ’เน€เธยเน€เธเธ Owner เน€เธยเน€เธเธ…เน€เธเธ Renter (public)
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
// Reviewee (เน€เธยเน€เธเธเน€เธยเน€เธโ€“เน€เธเธเน€เธยเน€เธเธเน€เธเธ•เน€เธเธเน€เธเธ”เน€เธเธ) เน€เธโ€ขเน€เธเธเน€เธยเน€เธยเน€เธเธ…เน€เธเธ‘เน€เธยเน€เธยเน€เธโ€เน€เธยเน€เธยเน€เธเธเน€เธเธ‘เน€เธยเน€เธยเน€เธโฌเน€เธโ€เน€เธเธ•เน€เธเธเน€เธเธ
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

	// เน€เธโ€ขเน€เธเธเน€เธเธเน€เธยเน€เธเธเน€เธยเน€เธเธ’ currentUser เน€เธโฌเน€เธยเน€เธยเน€เธย reviewee เน€เธยเน€เธเธเน€เธยเน€เธเธเน€เธเธ•เน€เธเธเน€เธเธ”เน€เธเธเน€เธยเน€เธเธ•เน€เธย
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
// เน€เธโ€เน€เธเธ–เน€เธยเน€เธยเน€เธยเน€เธเธเน€เธเธเน€เธเธเน€เธเธ…เน€เธยเน€เธยเน€เธเธเน€เธยเน€เธยเน€เธเธ…เน€เธยเน€เธยเน€เธเธเน€เธยเน€เธโฌเน€เธยเน€เธยเน€เธเธ’ เน€เธยเน€เธเธเน€เธยเน€เธเธเน€เธเธ stats เน€เธยเน€เธเธ…เน€เธเธเน€เธเธเน€เธเธ•เน€เธเธเน€เธเธ”เน€เธเธเน€เธโ€”เน€เธเธ•เน€เธยเน€เธยเน€เธโ€เน€เธยเน€เธเธเน€เธเธ‘เน€เธยเน€เธยเน€เธยเน€เธยเน€เธเธ’เน€เธยเน€เธเธเน€เธยเน€เธเธเน€เธยเน€เธโฌเน€เธยเน€เธยเน€เธเธ’ (public)
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

	// เนโ€โฌเนโ€โฌ 1. เน€เธโ€เน€เธเธ–เน€เธยเน€เธยเน€เธยเน€เธเธเน€เธเธเน€เธเธเน€เธเธ…เน€เธยเน€เธเธเน€เธยเน€เธยเน€เธยเน€เธยเน€เธยเน€เธเธ’เน€เธย AppUser + Renter เนโ€โฌเนโ€โฌ
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

	// เนโ€โฌเนโ€โฌ 2. Stats: totalCompletedRentals เนโ€โฌเนโ€โฌ
	var totalCompleted int
	rc.DB.QueryRow(`
		SELECT COUNT(*) FROM RentalRequest
		WHERE RenterUserId = $1 AND Status = 'Rental Completed'
	`, userID).Scan(&totalCompleted)

	// เนโ€โฌเนโ€โฌ 3. Stats: averageRating + reviewCount (owner reviewed renter) เนโ€โฌเนโ€โฌ
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

	// เนโ€โฌเนโ€โฌ 4. Reviews list เนโ€โฌเนโ€โฌ
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
			ur.CreatedAt,
			COALESCE(ur.RatingCommunication, 0) AS RatingCommunication,
			COALESCE(ur.RatingPunctuality,   0) AS RatingPunctuality,
			COALESCE(ur.RatingAccuracy,      0) AS RatingAccuracy,
			COALESCE(ur.RatingCare,          0) AS RatingCare
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
		RatingCommunication int `json:"ratingCommunication"`
		RatingPunctuality   int `json:"ratingPunctuality"`
		RatingAccuracy      int `json:"ratingAccuracy"`
		RatingCare          int `json:"ratingCare"`
	}

	var reviews []ReviewItem
	for rows.Next() {
		var item ReviewItem
		if err := rows.Scan(
			&item.ReviewID, &item.Rating, &item.ReviewText,
			&item.DeviceName, &item.ReviewerName, &item.CreatedAt,
			&item.RatingCommunication, &item.RatingPunctuality, &item.RatingAccuracy, &item.RatingCare,
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
