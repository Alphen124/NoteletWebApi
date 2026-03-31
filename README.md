# Notelet Backend Service

REST API backend สำหรับระบบเช่าอุปกรณ์ NoteLet พัฒนาด้วย Go 1.24  
รองรับการเช่าอุปกรณ์ (Notebook, MacBook, และอื่นๆ) ระหว่างนักศึกษา KMITL

## 📋 Tech Stack

| ส่วนประกอบ | รายละเอียด |
|---|---|
| Language | Go 1.24 |
| HTTP Server | `net/http` (built-in) |
| Database | PostgreSQL |
| Authentication | JWT (HS256), Google OAuth2, Firebase, Supabase |
| Real-time | WebSocket (`gorilla/websocket`) |
| File Storage | Local filesystem (`./uploads/`) |
| Deployment | Railway / Docker |

## 📂 โครงสร้าง Directory

```
NoteletWebApi/
├── main.go                    # Entry point — init services, run migrations, start server
├── go.mod                     # Go module dependencies
├── docker-compose.yml         # Docker Compose config
├── Dockerfile                 # Container build instructions
├── railway.json               # Railway deployment config
├── update_device_schema.sql   # One-off schema patch (เพิ่ม Condition column)
│
├── config/
│   └── database/
│       └── database.go        # PostgreSQL singleton connection (รองรับ DATABASE_URL และ env vars)
│
├── controllers/               # HTTP request handlers
│   ├── auth.go                # Register, Login, RefreshToken, Profile, AdminRegister
│   ├── device.go              # Device CRUD, status management, history
│   ├── oauth.go               # Google OAuth2 login/callback
│   ├── firebase_auth.go       # Firebase ID token → app JWT
│   ├── supabase_auth.go       # Supabase access token → app JWT
│   ├── rental_request.go      # Rental lifecycle management
│   ├── review.go              # Device reviews + user-to-user reviews
│   ├── chat.go                # WebSocket chat + notifications
│   └── upload.go              # Image upload handler
│
├── middlewares/
│   └── auth.go                # AuthMiddleware, KMITLEmailMiddleware, CORSMiddleware
│
├── models/
│   ├── user.go                # AppUser, Owner, Renter structs
│   └── device.go              # Device, DeviceType structs
│
├── routers/
│   └── router.go              # SetupRoutes — ผูก path กับ handler ทั้งหมด
│
├── services/
│   ├── jwt/jwt.go             # GenerateTokenPair, ValidateAccessToken, ValidateRefreshToken
│   ├── oauth/google.go        # InitGoogleOAuth, GetGoogleUserInfo
│   └── firebase/firebase.go   # Firebase Admin SDK initialization
│
├── types/
│   ├── requests/
│   │   ├── auth_request.go    # RegisterRequest, LoginRequest, RefreshTokenRequest
│   │   └── device_request.go  # CreateDeviceRequest, UpdateDeviceStatusRequest
│   └── responses/
│       └── response.go        # SuccessResponse, ErrorResponse, AuthResponse, DualRoleUserResponse
│
├── utils/
│   ├── password.go            # HashPassword, CheckPasswordHash (bcrypt)
│   └── email.go               # IsKMITLEmail
│
├── migrations/                # SQL migration scripts (001–009)
└── uploads/
    └── chat/                  # ไฟล์รูปภาพที่อัปโหลดผ่าน chat
```

## 🚀 Getting Started

### ข้อกำหนด
- Go 1.21+
- PostgreSQL 14+

### 1. ติดตั้ง dependencies
```bash
go mod download
```

### 2. ตั้งค่า environment variables
สร้างไฟล์ `.env` ที่ root:
```env
# Database
DATABASE_URL=postgres://user:password@host:5432/notelet?sslmode=require
# หรือใช้แยกตัวแปร (local dev)
DB_HOST=localhost
DB_PORT=5432
DB_USER=alphen
DB_PASSWORD=your_password
DB_NAME=notelet

# JWT
JWT_ACCESS_SECRET=your_access_secret_here
JWT_REFRESH_SECRET=your_refresh_secret_here

# Google OAuth
GOOGLE_CLIENT_ID=your_google_client_id
GOOGLE_CLIENT_SECRET=your_google_client_secret
GOOGLE_REDIRECT_URL=http://localhost:3001/api/auth/google/callback

# Admin
ADMIN_SECRET=notelet-admin-secret-2026

# Firebase (optional)
# วาง serviceAccountKey.json ไว้ใน root หรือตั้ง FIREBASE_CREDENTIALS
```

### 3. รัน server
```bash
go run main.go
# Server เริ่มต้นที่ http://localhost:3001
```

### 4. Hot reload (development)
```bash
go install github.com/cosmtrek/air@latest
air
```

## 🗄️ Database

ระบบใช้ PostgreSQL โดย migration จะรันอัตโนมัติเมื่อ start server  
สำหรับ migration ทั้งหมดดูที่ `migrations/` folder (001–009)

```bash
# รัน migration แยก (ถ้าต้องการ)
psql -U postgres -d notelet_db -f migrations/001_initialize_status_system.sql
```

## 📡 API Endpoints Summary

| กลุ่ม | Base Path | จำนวน Endpoints |
|---|---|---|
| Authentication | `/api/auth/` | 5 |
| Google OAuth | `/api/auth/google` | 2 |
| Firebase / Supabase | `/api/auth/firebase`, `/api/auth/supabase` | 2 |
| Device Management | `/api/devices/` | 9 |
| Rental Requests | `/api/rental-requests/` | 10 |
| Reviews | `/api/devices/{id}/reviews`, `/api/reviews/`, `/api/users/` | 7 |
| Chat & Notifications | `/api/chat/` | 9 |
| File Upload | `/api/upload/` | 1 |
| Health Check | `/api/health` | 1 |

ดูรายละเอียดทั้งหมดที่ [API_DEVICE_DOCUMENTATION.md](./API_DEVICE_DOCUMENTATION.md)

## 🔐 Authentication Flow

### Email/Password
```
POST /api/auth/register → POST /api/auth/login → Access Token (24h) + Refresh Token (7d)
POST /api/auth/refresh  → Access Token ใหม่
```

### Google OAuth
```
POST /api/auth/google → redirect → GET /api/auth/google/callback → JWT
```

### Firebase / Supabase
```
POST /api/auth/firebase  { id_token: "..." }     → JWT
POST /api/auth/supabase  { access_token: "..." } → JWT
```

> ทุก protected route ต้องส่ง `Authorization: Bearer <access_token>` ใน header

## 🐳 Docker

```bash
# รันด้วย Docker Compose
docker-compose up

# Build image เดี่ยว
docker build -t notelet-api .
docker run -p 3001:3001 --env-file .env notelet-api
```

## 🚂 Deploy บน Railway

ใช้ไฟล์ `railway.json` ในการ deploy อัตโนมัติ  
ตั้งค่า environment variables ใน Railway Dashboard และตั้ง `DATABASE_URL` ให้ชี้ไปที่ PostgreSQL service
