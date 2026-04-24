package middlewares

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"noteletwebservice-development/services/jwt"
	"noteletwebservice-development/types/responses"
	"noteletwebservice-development/utils"
)

// ContextKey สำหรับเก็บข้อมูล user ใน context
type ContextKey string

const UserContextKey ContextKey = "user"

// UserContext โครงสร้างสำหรับเก็บข้อมูล user ใน context
type UserContext struct {
	UserId             int
	Email              string
	IsAdmin            bool
	IsAuthorizedLender bool
	IsCentralStaff     bool
}

// AuthMiddleware middleware สำหรับตรวจสอบ JWT token
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ดึง Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondWithError(w, http.StatusUnauthorized, "Authorization header required")
			return
		}

		// ตรวจสอบรูปแบบ Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			respondWithError(w, http.StatusUnauthorized, "Invalid authorization header format")
			return
		}

		tokenString := parts[1]

		// Validate token
		claims, err := jwt.ValidateAccessToken(tokenString)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Invalid or expired token")
			return
		}

		// เก็บข้อมูล user ลง context
		userCtx := UserContext{
			UserId:             claims.UserId,
			Email:              claims.Email,
			IsAdmin:            claims.IsAdmin,
			IsAuthorizedLender: claims.IsAuthorizedLender,
			IsCentralStaff:     claims.IsCentralStaff,
		}
		ctx := context.WithValue(r.Context(), UserContextKey, userCtx)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AdminMiddleware middleware สำหรับตรวจสอบว่าผู้ใช้เป็น admin
func AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userCtx, ok := r.Context().Value(UserContextKey).(UserContext)
		if !ok {
			respondWithError(w, http.StatusUnauthorized, "User context not found")
			return
		}
		if !userCtx.IsAdmin {
			respondWithError(w, http.StatusForbidden, "Admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// KMITLEmailMiddleware middleware สำหรับตรวจสอบว่าเป็นอีเมล @kmitl.ac.th
func KMITLEmailMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userCtx, ok := r.Context().Value(UserContextKey).(UserContext)
		if !ok {
			respondWithError(w, http.StatusUnauthorized, "User context not found")
			return
		}

		if !utils.IsKMITLEmail(userCtx.Email) {
			respondWithError(w, http.StatusForbidden, "Only @kmitl.ac.th email addresses are allowed")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// CORS middleware สำหรับอนุญาต CORS
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// respondWithError ฟังก์ชันช่วยสำหรับส่ง error response
func respondWithError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(responses.ErrorResponse{
		Success: false,
		Message: message,
	})
}

// GetUserFromContext ดึงข้อมูล user จาก context
func GetUserFromContext(r *http.Request) (*UserContext, bool) {
	userCtx, ok := r.Context().Value(UserContextKey).(UserContext)
	return &userCtx, ok
}
