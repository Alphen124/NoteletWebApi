# types/

Type definitions สำหรับ request input และ response output ของ API

## 📋 โครงสร้างไฟล์

```
types/
├── requests/
│   ├── auth_request.go      # Login, Register, RefreshToken
│   └── device_request.go    # CreateDevice, UpdateDeviceStatus
└── responses/
    └── response.go          # SuccessResponse, ErrorResponse, AuthResponse, UserResponse
```

---

## requests/auth_request.go

### `RegisterRequest`
ใช้ใน: `POST /api/auth/register` และ `POST /api/admin/register`
```go
type RegisterRequest struct {
    Email    string `json:"email"    validate:"required,email"`
    Password string `json:"password" validate:"required,min=6"`
    FName    string `json:"fname"    validate:"required"`
    LName    string `json:"lname"    validate:"required"`
    Tel      string `json:"tel"      validate:"required"`
}
```

### `LoginRequest`
ใช้ใน: `POST /api/auth/login`
```go
type LoginRequest struct {
    Email    string `json:"email"    validate:"required,email"`
    Password string `json:"password" validate:"required"`
}
```

### `RefreshTokenRequest`
ใช้ใน: `POST /api/auth/refresh`
```go
type RefreshTokenRequest struct {
    RefreshToken string `json:"refresh_token" validate:"required"`
}
```

---

## requests/device_request.go

### `CreateDeviceRequest`
ใช้ใน: `POST /api/devices` และ `PUT /api/devices/{id}`
```go
type CreateDeviceRequest struct {
    Name        string  `json:"name"`
    Type        string  `json:"type"`        // Notebook | MacBook | Other
    Description string  `json:"description"`
    ImageUrl    string  `json:"imageUrl"`
    Price       float64 `json:"price"`
    Condition   string  `json:"condition"`   // new | like-new | good | fair | poor
}
```

### `UpdateDeviceStatusRequest`
ใช้ใน: `PATCH /api/devices/{id}/status`
```go
type UpdateDeviceStatusRequest struct {
    Status string `json:"status"`
}
```

---

## responses/response.go

### `SuccessResponse`
```go
type SuccessResponse struct {
    Success bool        `json:"success"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}
```

### `ErrorResponse`
```go
type ErrorResponse struct {
    Success bool   `json:"success"`
    Message string `json:"message"`
    Error   string `json:"error,omitempty"`
}
```
ใช้ใน: `middlewares/auth.go` และ `controllers/auth.go` เป็นต้น

### `AuthResponse`
```go
type AuthResponse struct {
    AccessToken  string      `json:"access_token"`
    RefreshToken string      `json:"refresh_token"`
    User         interface{} `json:"user"`
}
```
ใช้ใน: ทุก login/register response

### `UserResponse`
```go
type UserResponse struct {
    UserId   int    `json:"user_id"`
    Email    string `json:"email"`
    IsActive bool   `json:"is_active"`
    FName    string `json:"fname,omitempty"`
    LName    string `json:"lname,omitempty"`
    Tel      string `json:"tel,omitempty"`
}
```
ใช้ใน: `Register` response เท่านั้น

### `DualRoleUserResponse`
```go
type DualRoleUserResponse struct {
    UserId       int    `json:"user_id"`
    Email        string `json:"email"`
    IsActive     bool   `json:"is_active"`
    IsAdmin      bool   `json:"is_admin"`
    FName        string `json:"fname,omitempty"`
    LName        string `json:"lname,omitempty"`
    Tel          string `json:"tel,omitempty"`
    OwnerNo      int    `json:"owner_no,omitempty"`
    OwnerRating  int    `json:"owner_rating,omitempty"`
    RenterNo     int    `json:"renter_no,omitempty"`
    RenterRating int    `json:"renter_rating,omitempty"`
}
```
ใช้ใน: `Login`, `GetProfile`, `AdminRegister`, OAuth/Firebase/Supabase login response
