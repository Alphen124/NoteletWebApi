# routers/

กำหนด URL routes ทั้งหมดและผูก middleware กับ handler

## 📋 โครงสร้างไฟล์

```
routers/
└── router.go    # SetupRoutes() — ผูก path ทุกเส้นทาง
```

---

## router.go

### `SetupRoutes(...)` ฟังก์ชัน
รับ controller instances ทุกตัว คืน `*http.ServeMux` ที่ตั้งค่าครบ

```go
func SetupRoutes(
    authController      *controllers.AuthController,
    oauthController     *controllers.OAuthController,
    firebaseController  *controllers.FirebaseAuthController,
    supabaseController  *controllers.SupabaseAuthController,
    deviceController    *controllers.DeviceController,
    uploadController    *controllers.UploadController,
    reviewController    *controllers.ReviewController,
    rentalController    *controllers.RentalController,
    chatController      *controllers.ChatController,
) *http.ServeMux
```

---

## Route Table แบบละเอียด

### 🔓 Public Routes (ไม่ต้อง Auth)

| Method | Path | Handler |
|---|---|---|
| POST | `/api/auth/register` | `authController.Register` |
| POST | `/api/auth/login` | `authController.Login` |
| POST | `/api/auth/refresh` | `authController.RefreshToken` |
| POST | `/api/auth/google` | `oauthController.GoogleLogin` |
| GET | `/api/auth/google/callback` | `oauthController.GoogleCallback` |
| POST | `/api/auth/firebase` | `firebaseController.FirebaseLogin` |
| POST | `/api/auth/supabase` | `supabaseController.SupabaseLogin` |
| GET | `/api/devices/browse` | `deviceController.GetAllDevices` |
| GET | `/api/devices/{id}` | `deviceController.GetDevice` |
| GET | `/api/devices/{id}/reviews` | `reviewController.GetDeviceReviews` |
| GET | `/api/users/{id}/reviews` | `reviewController.GetUserReviews` |
| GET | `/api/users/{id}/rating` | `reviewController.GetUserRating` |
| GET | `/api/health` | inline handler |

### 🔒 Protected Routes (ต้องมี JWT)

| Method | Path | Handler |
|---|---|---|
| GET | `/api/auth/profile` | `authController.GetProfile` |
| GET | `/api/devices` | `deviceController.GetAllDevicesAdmin` |
| POST | `/api/devices` | `deviceController.CreateDevice` |
| GET | `/api/devices/my` | `deviceController.GetMyDevices` |
| PUT | `/api/devices/{id}` | `deviceController.UpdateDevice` |
| DELETE | `/api/devices/{id}` | `deviceController.DeleteDevice` |
| PATCH | `/api/devices/{id}/status` | `deviceController.UpdateDeviceStatus` |
| GET | `/api/devices/{id}/history` | `deviceController.GetDeviceStatusHistory` |
| POST | `/api/devices/{id}/reviews` | `reviewController.CreateDeviceReview` |
| POST | `/api/upload/images` | `uploadController.UploadImages` |
| POST | `/api/rental-requests` | `rentalController.CreateRentalRequest` |
| GET | `/api/rental-requests/incoming` | `rentalController.GetIncomingRequests` |
| GET | `/api/rental-requests/outgoing` | `rentalController.GetOutgoingRequests` |
| GET | `/api/rental-requests/{id}` | `rentalController.GetRentalRequest` |
| PATCH | `/api/rental-requests/{id}/confirm` | `rentalController.ConfirmRequest` |
| PATCH | `/api/rental-requests/{id}/reject` | `rentalController.RejectRequest` |
| PATCH | `/api/rental-requests/{id}/active` | `rentalController.MarkActive` |
| PATCH | `/api/rental-requests/{id}/returned` | `rentalController.MarkReturned` |
| PATCH | `/api/rental-requests/{id}/cancel` | `rentalController.CancelRequest` |
| PATCH | `/api/rental-requests/{id}/update-dates` | `rentalController.UpdateRequestDates` |
| POST | `/api/reviews` | `reviewController.CreateUserReview` |
| GET | `/api/reviews/eligibility` | `reviewController.CheckReviewEligibility` |
| PATCH | `/api/reviews/{reviewNo}/reply` | `reviewController.ReplyToReview` |
| POST | `/api/chat/confirm-rental` | `rentalController.ConfirmRentalFromChat` |
| GET (WS) | `/api/chat/ws` | `chatController.ServeWS` |
| GET | `/api/chat/rooms` | `chatController.GetRooms` |
| POST | `/api/chat/device-room` | `chatController.EnsureDeviceRoom` |
| POST | `/api/chat/upload-image` | `chatController.UploadChatImage` |
| GET | `/api/chat/notifications` | `chatController.GetNotifications` |
| GET | `/api/chat/notifications/unread` | `chatController.GetUnreadCount` |
| PATCH | `/api/chat/notifications/read` | `chatController.MarkNotificationsRead` |
| GET | `/api/chat/owner-rooms` | `chatController.GetOwnerRooms` |

### 🛡️ Admin Route (X-Admin-Secret header)

| Method | Path | Handler |
|---|---|---|
| POST | `/api/admin/register` | `authController.AdminRegister` |

---

## Middleware Stack

```
[Public]     →  CORSMiddleware → Handler
[Protected]  →  CORSMiddleware → AuthMiddleware → Handler
[Profile]    →  CORSMiddleware → AuthMiddleware → KMITLEmailMiddleware → Handler
[WebSocket]  →  JWT ใน query param (?token=...) → Handler
```

> `/api/auth/profile` เป็นเส้นเดียวที่ใช้ `KMITLEmailMiddleware`
