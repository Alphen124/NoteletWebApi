package routers

import (
	"net/http"
	"strings"

	"noteletwebservice-development/controllers"
	"noteletwebservice-development/middlewares"
)

// SetupRoutes กำหนด routes สำหรับ API
func SetupRoutes(authController *controllers.AuthController, oauthController *controllers.OAuthController, firebaseController *controllers.FirebaseAuthController, supabaseController *controllers.SupabaseAuthController, deviceController *controllers.DeviceController, uploadController *controllers.UploadController, reviewController *controllers.ReviewController, rentalController *controllers.RentalController, chatController *controllers.ChatController) *http.ServeMux {
	mux := http.NewServeMux()

	// Public routes (ไม่ต้องการ authentication)
	mux.HandleFunc("/api/auth/register", authController.Register)
	mux.HandleFunc("/api/auth/login", authController.Login)
	mux.HandleFunc("/api/auth/refresh", authController.RefreshToken)

	// Admin register route (requires X-Admin-Secret header)
	mux.HandleFunc("/api/admin/register", middlewares.CORSMiddleware(http.HandlerFunc(authController.AdminRegister)).ServeHTTP)

	// OAuth routes
	mux.HandleFunc("/api/auth/google", oauthController.GoogleLogin)
	mux.HandleFunc("/api/auth/google/callback", oauthController.GoogleCallback)

	// Firebase Authentication route (POST id_token → returns app JWT)
	mux.HandleFunc("/api/auth/firebase", firebaseController.FirebaseLogin)

	// Supabase Google Auth route (POST access_token → returns app JWT)
	mux.HandleFunc("/api/auth/supabase", supabaseController.SupabaseLogin)

	// Public device routes (can browse without login)
	mux.HandleFunc("/api/devices/browse", deviceController.GetAllDevices)

	// Protected routes (ต้องการ authentication)
	// Profile
	protectedMux := http.NewServeMux()
	protectedMux.HandleFunc("/api/auth/profile", authController.GetProfile)

	mux.Handle("/api/auth/profile",
		middlewares.CORSMiddleware(
			middlewares.AuthMiddleware(
				middlewares.KMITLEmailMiddleware(protectedMux),
			),
		),
	)

	// Device management routes (protected) - includes review POST
	deviceMux := http.NewServeMux()
	deviceMux.HandleFunc("/api/devices", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			deviceController.GetAllDevicesAdmin(w, r)
		} else {
			deviceController.CreateDevice(w, r)
		}
	})
	deviceMux.HandleFunc("/api/devices/my", deviceController.GetMyDevices)
	deviceMux.HandleFunc("/api/devices/", func(w http.ResponseWriter, r *http.Request) {
		// Extract device ID from path
		path := r.URL.Path
		if path == "/api/devices/" {
			http.Error(w, "Device ID is required", http.StatusBadRequest)
			return
		}

		if len(path) > len("/api/devices/") {
			parts := path[len("/api/devices/"):]
			// Check for /reviews endpoint (POST protected, GET public handled below)
			if strings.HasSuffix(parts, "/reviews") && r.Method == http.MethodPost {
				reviewController.CreateDeviceReview(w, r)
				return
			}
			// Check for /status endpoint (PATCH)
			if len(parts) > 7 && parts[len(parts)-7:] == "/status" && r.Method == http.MethodPatch {
				deviceController.UpdateDeviceStatus(w, r)
				return
			}
			// Check for /history endpoint (GET)
			if len(parts) > 8 && parts[len(parts)-8:] == "/history" && r.Method == http.MethodGet {
				deviceController.GetDeviceStatusHistory(w, r)
				return
			}
		}

		// Handle regular device operations
		if r.Method == http.MethodGet {
			deviceController.GetDevice(w, r)
		} else if r.Method == http.MethodPut {
			deviceController.UpdateDevice(w, r)
		} else if r.Method == http.MethodDelete {
			deviceController.DeleteDevice(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.Handle("/api/devices", middlewares.CORSMiddleware(middlewares.AuthMiddleware(deviceMux)))
	mux.Handle("/api/devices/my", middlewares.CORSMiddleware(middlewares.AuthMiddleware(deviceMux)))

	// /api/devices/{id}/... routes: GET single device & GET reviews are public, everything else protected
	mux.HandleFunc("/api/devices/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		suffix := path[len("/api/devices/"):]

		// Public: GET /api/devices/{id}/reviews
		if strings.HasSuffix(suffix, "/reviews") && r.Method == http.MethodGet {
			middlewares.CORSMiddleware(http.HandlerFunc(reviewController.GetDeviceReviews)).ServeHTTP(w, r)
			return
		}
		// Public: GET /api/devices/{id} (no sub-path, just a numeric id)
		if r.Method == http.MethodGet && !strings.Contains(suffix, "/") {
			middlewares.CORSMiddleware(http.HandlerFunc(deviceController.GetDevice)).ServeHTTP(w, r)
			return
		}
		// All other /api/devices/{id}/... requests go through auth middleware
		middlewares.CORSMiddleware(middlewares.AuthMiddleware(deviceMux)).ServeHTTP(w, r)
	})

	// Upload routes (protected)
	uploadMux := http.NewServeMux()
	uploadMux.HandleFunc("/api/upload/images", uploadController.UploadImages)
	mux.Handle("/api/upload/images", middlewares.CORSMiddleware(middlewares.AuthMiddleware(uploadMux)))

	// Rental request routes (all protected)
	rentalMux := http.NewServeMux()
	rentalMux.HandleFunc("/api/rental-requests", rentalController.CreateRentalRequest)
	rentalMux.HandleFunc("/api/rental-requests/incoming", rentalController.GetIncomingRequests)
	rentalMux.HandleFunc("/api/rental-requests/outgoing", rentalController.GetOutgoingRequests)
	rentalMux.HandleFunc("/api/chat/confirm-rental", rentalController.ConfirmRentalFromChat)
	rentalMux.HandleFunc("/api/rental-requests/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/confirm") && r.Method == http.MethodPatch {
			rentalController.ConfirmRequest(w, r)
		} else if strings.HasSuffix(path, "/reject") && r.Method == http.MethodPatch {
			rentalController.RejectRequest(w, r)
		} else if strings.HasSuffix(path, "/active") && r.Method == http.MethodPatch {
			rentalController.MarkActive(w, r)
		} else if strings.HasSuffix(path, "/returned") && r.Method == http.MethodPatch {
			rentalController.MarkReturned(w, r)
		} else if strings.HasSuffix(path, "/cancel") && r.Method == http.MethodPatch {
			rentalController.CancelRequest(w, r)
		} else if strings.HasSuffix(path, "/update-dates") && r.Method == http.MethodPatch {
			rentalController.UpdateRequestDates(w, r)
		} else if r.Method == http.MethodGet {
			rentalController.GetRentalRequest(w, r)
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
	})
	mux.Handle("/api/rental-requests", middlewares.CORSMiddleware(middlewares.AuthMiddleware(rentalMux)))
	mux.Handle("/api/rental-requests/", middlewares.CORSMiddleware(middlewares.AuthMiddleware(rentalMux)))
	mux.Handle("/api/chat/confirm-rental", middlewares.CORSMiddleware(middlewares.AuthMiddleware(rentalMux)))

	// Health check endpoint
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","message":"NoteLet Web Service is running"}`))
	})

	// ─── Chat routes ───────────────────────────────────────────────
	// WebSocket endpoint: /api/chat/ws?token=<jwt>&room=<name>  (auth via query param)
	mux.HandleFunc("/api/chat/ws", chatController.ServeWS)
	// REST: protected chat helpers
	chatMux := http.NewServeMux()
	chatMux.HandleFunc("/api/chat/rooms", chatController.GetRooms)
	chatMux.HandleFunc("/api/chat/device-room", chatController.EnsureDeviceRoom)
	chatMux.HandleFunc("/api/chat/upload-image", chatController.UploadChatImage)
	chatMux.HandleFunc("/api/chat/notifications", chatController.GetNotifications)
	chatMux.HandleFunc("/api/chat/notifications/unread", chatController.GetUnreadCount)
	chatMux.HandleFunc("/api/chat/notifications/read", chatController.MarkNotificationsRead)
	chatMux.HandleFunc("/api/chat/owner-rooms", chatController.GetOwnerRooms)
	mux.Handle("/api/chat/rooms", middlewares.CORSMiddleware(middlewares.AuthMiddleware(chatMux)))
	mux.Handle("/api/chat/device-room", middlewares.CORSMiddleware(middlewares.AuthMiddleware(chatMux)))
	mux.Handle("/api/chat/upload-image", middlewares.CORSMiddleware(middlewares.AuthMiddleware(chatMux)))
	mux.Handle("/api/chat/notifications", middlewares.CORSMiddleware(middlewares.AuthMiddleware(chatMux)))
	mux.Handle("/api/chat/notifications/unread", middlewares.CORSMiddleware(middlewares.AuthMiddleware(chatMux)))
	mux.Handle("/api/chat/notifications/read", middlewares.CORSMiddleware(middlewares.AuthMiddleware(chatMux)))
	mux.Handle("/api/chat/owner-rooms", middlewares.CORSMiddleware(middlewares.AuthMiddleware(chatMux)))

	// ─── User-to-User Review routes ──────────────────────────────────────────
	reviewApiMux := http.NewServeMux()
	// POST /api/reviews              → สร้างรีวิว (ตรวจ role + status อัตโนมัติ)
	// GET  /api/reviews/eligibility  → ตรวจสิทธิ์รีวิวก่อนแสดงปุ่ม
	reviewApiMux.HandleFunc("/api/reviews", reviewController.CreateUserReview)
	reviewApiMux.HandleFunc("/api/reviews/eligibility", reviewController.CheckReviewEligibility)
	// PATCH /api/reviews/{reviewNo}/reply → ตอบกลับรีวิว (reviewee เท่านั้น)
	reviewApiMux.HandleFunc("/api/reviews/", reviewController.ReplyToReview)
	mux.Handle("/api/reviews", middlewares.CORSMiddleware(middlewares.AuthMiddleware(reviewApiMux)))
	mux.Handle("/api/reviews/", middlewares.CORSMiddleware(middlewares.AuthMiddleware(reviewApiMux)))

	// ─── User profile / rating routes (public GET) ───────────────────────────
	// GET /api/users/{userId}/reviews → ดูรีวิวทั้งหมดของ user
	// GET /api/users/{userId}/rating  → ดู avg rating ของ user
	mux.HandleFunc("/api/users/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/reviews") && r.Method == http.MethodGet {
			middlewares.CORSMiddleware(http.HandlerFunc(reviewController.GetUserReviews)).ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(path, "/rating") && r.Method == http.MethodGet {
			middlewares.CORSMiddleware(http.HandlerFunc(reviewController.GetUserRating)).ServeHTTP(w, r)
			return
		}
		http.Error(w, "Not found", http.StatusNotFound)
	})

	return mux
}

// ApplyCORS wrapper สำหรับใช้ CORS middleware กับ mux ทั้งหมด
func ApplyCORS(mux *http.ServeMux) http.Handler {
	return middlewares.CORSMiddleware(mux)
}
