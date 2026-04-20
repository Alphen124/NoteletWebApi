package controllers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"noteletwebservice-development/models"
	jwtSvc "noteletwebservice-development/services/jwt"
	"noteletwebservice-development/types/responses"
	"noteletwebservice-development/utils"

	"github.com/golang-jwt/jwt/v5"
)

// SupabaseAuthController handles authentication via Supabase Google OAuth.
type SupabaseAuthController struct {
	DB           *sql.DB
	jwksCache    map[string]*ecdsa.PublicKey
	jwksCachedAt time.Time
	jwksMu       sync.RWMutex
}

// NewSupabaseAuthController creates a new SupabaseAuthController.
func NewSupabaseAuthController(db *sql.DB) *SupabaseAuthController {
	return &SupabaseAuthController{DB: db}
}

type supabaseLoginRequest struct {
	AccessToken string `json:"access_token"`
}

// jwkKeySet เก็บโครงสร้าง JWKS response จาก Supabase
type jwkKeySet struct {
	Keys []struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Alg string `json:"alg"`
		Crv string `json:"crv"`
		X   string `json:"x"`
		Y   string `json:"y"`
	} `json:"keys"`
}

// getPublicKey ดึง EC public key จาก Supabase JWKS endpoint (cached 1 ชั่วโมง)
func (sc *SupabaseAuthController) getPublicKey(kid string) (*ecdsa.PublicKey, error) {
	// ตรวจ cache ก่อน
	sc.jwksMu.RLock()
	if sc.jwksCache != nil && time.Since(sc.jwksCachedAt) < time.Hour {
		if key, ok := sc.jwksCache[kid]; ok {
			sc.jwksMu.RUnlock()
			return key, nil
		}
	}
	sc.jwksMu.RUnlock()

	// ดึง JWKS ใหม่จาก Supabase
	supabaseURL := os.Getenv("SUPABASE_URL")
	if supabaseURL == "" {
		return nil, fmt.Errorf("SUPABASE_URL not configured")
	}
	jwksURL := strings.TrimRight(supabaseURL, "/") + "/auth/v1/.well-known/jwks.json"

	resp, err := http.Get(jwksURL) // #nosec G107 — URL constructed from trusted env var
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %v", err)
	}
	defer resp.Body.Close()

	var keySet jwkKeySet
	if err := json.NewDecoder(resp.Body).Decode(&keySet); err != nil {
		return nil, fmt.Errorf("failed to parse JWKS: %v", err)
	}

	// แปลง JWK → *ecdsa.PublicKey แล้วเก็บ cache
	sc.jwksMu.Lock()
	sc.jwksCache = make(map[string]*ecdsa.PublicKey)
	sc.jwksCachedAt = time.Now()
	for _, k := range keySet.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" {
			continue
		}
		xBytes, err1 := base64.RawURLEncoding.DecodeString(k.X)
		yBytes, err2 := base64.RawURLEncoding.DecodeString(k.Y)
		if err1 != nil || err2 != nil {
			continue
		}
		sc.jwksCache[k.Kid] = &ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(xBytes),
			Y:     new(big.Int).SetBytes(yBytes),
		}
	}
	pubKey := sc.jwksCache[kid]
	sc.jwksMu.Unlock()

	if pubKey == nil {
		return nil, fmt.Errorf("no matching key found for kid: %s", kid)
	}
	return pubKey, nil
}

// SupabaseLogin verifies a Supabase access_token (JWT signed with ES256) obtained
// after Google sign-in on the frontend, then returns the app's own JWT pair.
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

	// Parse โดยยังไม่ verify เพื่อดึง kid จาก header
	unverified, _, err := jwt.NewParser().ParseUnverified(req.AccessToken, jwt.MapClaims{})
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Malformed token", "")
		return
	}
	kid, _ := unverified.Header["kid"].(string)

	// ดึง EC public key ที่ตรงกับ kid จาก Supabase JWKS
	pubKey, err := sc.getPublicKey(kid)
	if err != nil {
		log.Printf("[supabase_auth] JWKS error: %v", err)
		respondWithError(w, http.StatusUnauthorized, "Failed to fetch Supabase signing key", "")
		return
	}

	// Verify token ด้วย ES256 public key
	token, err := jwt.Parse(req.AccessToken, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pubKey, nil
	})
	if err != nil || !token.Valid {
		log.Printf("[supabase_auth] JWT verification failed: %v", err)
		respondWithError(w, http.StatusUnauthorized, "Invalid or expired Supabase token", "")
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Invalid token claims", "")
		return
	}

	// Extract email
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

	// Extract display name จาก user_metadata ที่ Supabase ดึงมาจาก Google
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
	accessToken, refreshToken, err := jwtSvc.GenerateTokenPair(user.UserId, user.Email, false, false, false)
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

	var userId int
	err = tx.QueryRow(`
		INSERT INTO appuser (email, passwordhash, isactive, createdat)
		VALUES ($1, NULL, true, NOW())
		RETURNING userid
	`, email).Scan(&userId)
	if err != nil {
		log.Printf("[supabase_auth] INSERT appuser failed: %v", err)
		return models.AppUser{}, err
	}

	var nextOwnerNo int
	tx.QueryRow(`SELECT COALESCE(MAX(ownerno), 0) + 1 FROM owner`).Scan(&nextOwnerNo)
	_, err = tx.Exec(`
		INSERT INTO owner (ownerno, name, fname, lname, tel, userid)
		VALUES ($1, $2, $3, $4, '', $5)
	`, nextOwnerNo, fullName, fname, lname, userId)
	if err != nil {
		log.Printf("[supabase_auth] INSERT owner failed (nextOwnerNo=%d): %v", nextOwnerNo, err)
		return models.AppUser{}, err
	}

	var nextRenterNo int
	tx.QueryRow(`SELECT COALESCE(MAX(renterno), 0) + 1 FROM renter`).Scan(&nextRenterNo)
	_, err = tx.Exec(`
		INSERT INTO renter (renterno, name, fname, lname, tel, userid)
		VALUES ($1, $2, $3, $4, '', $5)
	`, nextRenterNo, fullName, fname, lname, userId)
	if err != nil {
		log.Printf("[supabase_auth] INSERT renter failed (nextRenterNo=%d): %v", nextRenterNo, err)
		return models.AppUser{}, err
	}

	if err = tx.Commit(); err != nil {
		return models.AppUser{}, err
	}

	return models.AppUser{UserId: userId, Email: email, IsActive: true}, nil
}
