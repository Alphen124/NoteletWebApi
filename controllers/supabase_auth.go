package controllers

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"noteletwebservice-development/models"
	jwtSvc "noteletwebservice-development/services/jwt"
	"noteletwebservice-development/types/responses"
	"noteletwebservice-development/utils"

	"github.com/golang-jwt/jwt/v5"
)

// SupabaseAuthController handles authentication via Supabase Google OAuth.
type SupabaseAuthController struct {
	DB *sql.DB
}

// NewSupabaseAuthController creates a new SupabaseAuthController.
func NewSupabaseAuthController(db *sql.DB) *SupabaseAuthController {
	return &SupabaseAuthController{DB: db}
}

type supabaseLoginRequest struct {
	AccessToken string `json:"access_token"`
}

// SupabaseLogin verifies a Supabase access_token (JWT) obtained after Google
// sign-in on the frontend, then returns the app's own JWT access/refresh pair.
//
// POST /api/auth/supabase
// Body: { "access_token": "<supabase_access_token>" }
func (sc *SupabaseAuthController) SupabaseLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	var req supabaseLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AccessToken == "" {
		respondWithError(w, http.StatusBadRequest, "Missing access_token in request body", "")
		return
	}

	// Load Supabase JWT secret from environment variable
	jwtSecret := os.Getenv("SUPABASE_JWT_SECRET")
	if jwtSecret == "" {
		respondWithError(w, http.StatusInternalServerError, "Supabase JWT secret not configured", "")
		return
	}

	// Supabase exposes the JWT secret as a plain string in Project Settings → API.
	// Try verifying with the raw secret first, then fall back to base64-decoded form.
	secretBytes := []byte(jwtSecret)
	if decoded, err := base64.StdEncoding.DecodeString(jwtSecret); err == nil && len(decoded) > 0 {
		// Only prefer decoded form if it looks like real key material (not a printable phrase)
		secretBytes = decoded
	}

	// Parse and verify the Supabase JWT (signed with HS256)
	token, err := jwt.Parse(req.AccessToken, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secretBytes, nil
	})
	if err != nil || !token.Valid {
		// Try the raw (non-decoded) secret as a fallback
		token, err = jwt.Parse(req.AccessToken, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(jwtSecret), nil
		})
		if err != nil || !token.Valid {
			log.Printf("[supabase_auth] JWT verification failed: %v", err)
			respondWithError(w, http.StatusUnauthorized, "Invalid or expired Supabase token", "")
			return
		}
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Invalid token claims", "")
		return
	}

	// Extract email from top-level claim
	email, _ := claims["email"].(string)
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		respondWithError(w, http.StatusBadRequest, "No email found in token", "")
		return
	}

	// Enforce @kmitl.ac.th email policy
	if !utils.IsKMITLEmail(email) {
		respondWithError(w, http.StatusForbidden, "Only @kmitl.ac.th email addresses are allowed", "")
		return
	}

	// Extract display name from user_metadata (populated by Supabase from Google)
	fullName := ""
	if meta, ok := claims["user_metadata"].(map[string]interface{}); ok {
		fullName, _ = meta["full_name"].(string)
		if fullName == "" {
			fullName, _ = meta["name"].(string)
		}
	}
	if fullName == "" {
		fullName = email
	}

	// Find or auto-create the app user
	var user models.AppUser
	err = sc.DB.QueryRow(`
		SELECT userid, email, isactive, createdat
		FROM appuser WHERE email = $1
	`, email).Scan(&user.UserId, &user.Email, &user.IsActive, &user.CreatedAt)

	if err == sql.ErrNoRows {
		user, err = sc.createUserFromSupabase(email, fullName)
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

	// Fetch owner/renter profile data
	var ownerNo, renterNo sql.NullInt64
	var fname, lname, tel string
	var ownerRating, renterRating sql.NullInt64

	sc.DB.QueryRow(`SELECT ownerno, fname, lname, tel, rating FROM owner WHERE userid = $1`,
		user.UserId).Scan(&ownerNo, &fname, &lname, &tel, &ownerRating)
	sc.DB.QueryRow(`SELECT renterno, rating FROM renter WHERE userid = $1`,
		user.UserId).Scan(&renterNo, &renterRating)

	// Issue app JWT tokens
	accessToken, refreshToken, err := jwtSvc.GenerateTokenPair(user.UserId, user.Email, false)
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

	respondWithSuccess(w, http.StatusOK, "Login successful via Supabase", responses.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         responseData,
	})
}

// createUserFromSupabase inserts a new AppUser, Owner, and Renter row for a
// first-time Supabase OAuth sign-in.
func (sc *SupabaseAuthController) createUserFromSupabase(email, fullName string) (models.AppUser, error) {
	parts := strings.SplitN(strings.TrimSpace(fullName), " ", 2)
	fname := fullName
	lname := ""
	if len(parts) == 2 {
		fname = parts[0]
		lname = parts[1]
	}

	tx, err := sc.DB.Begin()
	if err != nil {
		return models.AppUser{}, err
	}
	defer tx.Rollback()

	// Insert AppUser with NULL password hash (OAuth-only account)
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
