package controllers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"noteletwebservice-development/models"
	firebasesvc "noteletwebservice-development/services/firebase"
	"noteletwebservice-development/services/jwt"
	"noteletwebservice-development/types/responses"
	"noteletwebservice-development/utils"
)

// FirebaseAuthController handles Firebase-based authentication.
type FirebaseAuthController struct {
	DB *sql.DB
}

// NewFirebaseAuthController creates a new FirebaseAuthController.
func NewFirebaseAuthController(db *sql.DB) *FirebaseAuthController {
	return &FirebaseAuthController{DB: db}
}

type firebaseLoginRequest struct {
	IDToken string `json:"id_token"`
}

// FirebaseLogin verifies a Firebase ID token sent from the frontend and returns
// the app's own JWT access/refresh token pair.
func (fc *FirebaseAuthController) FirebaseLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	var req firebaseLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IDToken == "" {
		respondWithError(w, http.StatusBadRequest, "Missing id_token in request body", "")
		return
	}

	// Verify Firebase ID token with Firebase Admin SDK
	token, err := firebasesvc.VerifyIDToken(context.Background(), req.IDToken)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid or expired Firebase token", "")
		return
	}

	// Extract standard claims from the verified token
	email, _ := token.Claims["email"].(string)
	emailVerified, _ := token.Claims["email_verified"].(bool)
	fullName, _ := token.Claims["name"].(string)
	email = strings.ToLower(strings.TrimSpace(email))

	if email == "" {
		respondWithError(w, http.StatusBadRequest, "No email found in token", "")
		return
	}

	// Enforce @kmitl.ac.th only
	if !utils.IsKMITLEmail(email) {
		respondWithError(w, http.StatusForbidden, "Only @kmitl.ac.th email addresses are allowed", "")
		return
	}

	if !emailVerified {
		respondWithError(w, http.StatusBadRequest, "Google account email is not verified", "")
		return
	}

	// Find or auto-create the app user
	var user models.AppUser
	err = fc.DB.QueryRow(`
		SELECT userid, email, isactive, createdat
		FROM appuser WHERE email = $1
	`, email).Scan(&user.UserId, &user.Email, &user.IsActive, &user.CreatedAt)

	if err == sql.ErrNoRows {
		user, err = fc.createUserFromFirebase(email, fullName)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to create user account", err.Error())
			return
		}
	} else if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}

	if !user.IsActive {
		respondWithError(w, http.StatusForbidden, "Account is inactive", "")
		return
	}

	// Fetch owner/renter data
	var ownerNo, renterNo sql.NullInt64
	var fname, lname, tel string
	var ownerRating, renterRating sql.NullInt64

	fc.DB.QueryRow(`SELECT ownerno, fname, lname, tel, rating FROM owner WHERE userid = $1`,
		user.UserId).Scan(&ownerNo, &fname, &lname, &tel, &ownerRating)
	fc.DB.QueryRow(`SELECT renterno, rating FROM renter WHERE userid = $1`,
		user.UserId).Scan(&renterNo, &renterRating)

	// Issue app JWT tokens
	accessToken, refreshToken, err := jwt.GenerateTokenPair(user.UserId, user.Email, false)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to generate tokens", err.Error())
		return
	}

	responseData := responses.DualRoleUserResponse{
		UserId:   user.UserId,
		Email:    user.Email,
		IsActive: user.IsActive,
		FName:    fname,
		LName:    lname,
		Tel:      tel,
	}
	if ownerNo.Valid {
		responseData.OwnerNo = int(ownerNo.Int64)
		if ownerRating.Valid {
			responseData.OwnerRating = int(ownerRating.Int64)
		}
	}
	if renterNo.Valid {
		responseData.RenterNo = int(renterNo.Int64)
		if renterRating.Valid {
			responseData.RenterRating = int(renterRating.Int64)
		}
	}

	respondWithSuccess(w, http.StatusOK, "Login successful via Firebase", responses.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         responseData,
	})
}

// createUserFromFirebase creates a new AppUser (+ owner + renter rows) from a
// Firebase-authenticated user who doesn't yet have an account in the database.
func (fc *FirebaseAuthController) createUserFromFirebase(email, fullName string) (models.AppUser, error) {
	parts := strings.SplitN(strings.TrimSpace(fullName), " ", 2)
	fname := fullName
	lname := ""
	if len(parts) == 2 {
		fname = parts[0]
		lname = parts[1]
	}

	tx, err := fc.DB.Begin()
	if err != nil {
		return models.AppUser{}, err
	}
	defer tx.Rollback()

	// Insert AppUser with NULL password (OAuth-only account)
	var userId int
	err = tx.QueryRow(`
		INSERT INTO appuser (email, passwordhash, isactive, createdat)
		VALUES ($1, NULL, true, NOW())
		RETURNING userid
	`, email).Scan(&userId)
	if err != nil {
		return models.AppUser{}, err
	}

	// Create Owner row
	var nextOwnerNo int
	tx.QueryRow(`SELECT COALESCE(MAX(ownerno), 0) + 1 FROM owner`).Scan(&nextOwnerNo)
	_, err = tx.Exec(`
		INSERT INTO owner (ownerno, name, fname, lname, tel, rating, userid)
		VALUES ($1, $2, $3, $4, '', 0, $5)
	`, nextOwnerNo, fullName, fname, lname, userId)
	if err != nil {
		return models.AppUser{}, err
	}

	// Create Renter row
	var nextRenterNo int
	tx.QueryRow(`SELECT COALESCE(MAX(renterno), 0) + 1 FROM renter`).Scan(&nextRenterNo)
	_, err = tx.Exec(`
		INSERT INTO renter (renterno, name, fname, lname, tel, rating, userid)
		VALUES ($1, $2, $3, $4, '', 0, $5)
	`, nextRenterNo, fullName, fname, lname, userId)
	if err != nil {
		return models.AppUser{}, err
	}

	if err = tx.Commit(); err != nil {
		return models.AppUser{}, err
	}
	return models.AppUser{UserId: userId, Email: email, IsActive: true}, nil
}
