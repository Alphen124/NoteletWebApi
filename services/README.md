# services/

บริการภายนอกและ logic สำหรับ JWT, OAuth, และ Firebase

## 📋 โครงสร้างไฟล์

```
services/
├── jwt/
│   └── jwt.go          # สร้าง/ตรวจสอบ JWT token
├── oauth/
│   └── google.go       # Google OAuth2 config และ user info
└── firebase/
    └── firebase.go     # Firebase Admin SDK initialization
```

---

## jwt/jwt.go

### ค่าเริ่มต้น Token

| ตัวแปร | ค่า | หมายเหตุ |
|---|---|---|
| `AccessTokenExpiry` | 24 ชั่วโมง | สำหรับ API requests |
| `RefreshTokenExpiry` | 7 วัน | สำหรับขอ access token ใหม่ |

### Claims Struct
```go
type Claims struct {
    UserId  int    `json:"user_id"`
    Email   string `json:"email"`
    IsAdmin bool   `json:"is_admin"`
    jwt.RegisteredClaims
}
```
Algorithm: **HS256**

### ฟังก์ชัน

| ฟังก์ชัน | Signature | หน้าที่ |
|---|---|---|
| `GenerateAccessToken` | `(userId int, email string, isAdmin bool) (string, error)` | สร้าง access token |
| `GenerateRefreshToken` | `(userId int, email string, isAdmin bool) (string, error)` | สร้าง refresh token |
| `GenerateTokenPair` | `(userId int, email string, isAdmin bool) (string, string, error)` | สร้างทั้งคู่ |
| `ValidateAccessToken` | `(tokenString string) (*Claims, error)` | ตรวจสอบ access token |
| `ValidateRefreshToken` | `(tokenString string) (*Claims, error)` | ตรวจสอบ refresh token |

### Environment Variables
```env
JWT_ACCESS_SECRET=...   # จำเป็น — app crash ทันทีถ้าไม่ตั้งค่า
JWT_REFRESH_SECRET=...  # จำเป็น — app crash ทันทีถ้าไม่ตั้งค่า
```

### ตัวอย่าง
```go
import jwtSvc "noteletwebservice-development/services/jwt"

// สร้าง token pair หลัง login
accessToken, refreshToken, err := jwtSvc.GenerateTokenPair(user.UserId, user.Email, user.IsAdmin)

// ตรวจสอบ token ใน middleware
claims, err := jwtSvc.ValidateAccessToken(tokenString)
```

---

## oauth/google.go

### ตัวแปร Global
```go
var GoogleOAuthConfig *oauth2.Config
```

### ฟังก์ชัน

| ฟังก์ชัน | หน้าที่ |
|---|---|
| `InitGoogleOAuth(clientID, clientSecret, redirectURL string)` | ตั้งค่า OAuth2 config (เรียกจาก `main.go`) |
| `GetGoogleUserInfo(accessToken string) (*GoogleUserInfo, error)` | ดึงข้อมูล user จาก Google API |

### Scopes ที่ดึง
- `userinfo.email`
- `userinfo.profile`

### `GoogleUserInfo` Struct
```go
type GoogleUserInfo struct {
    ID            string
    Email         string
    VerifiedEmail bool
    Name          string
    GivenName     string
    FamilyName    string
    Picture       string
    Locale        string
    HD            string  // Hosted domain เช่น kmitl.ac.th
}
```

### Environment Variables
```env
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
GOOGLE_REDIRECT_URL=http://localhost:3001/api/auth/google/callback
```

---

## firebase/firebase.go

### หน้าที่
- เริ่ม Firebase Admin SDK (เรียกจาก `main.go`)
- ใช้ยืนยัน Firebase ID token (จาก Google Sign-In) ก่อนออก app JWT

### การตั้งค่า
วางไฟล์ `serviceAccountKey.json` ไว้ที่ root ของโปรเจกต์  
(หรือตั้ง env `GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json`)

> ถ้าไม่มี key file — Firebase init จะ warn แต่ app ยังทำงานต่อ  
> `/api/auth/firebase` จะตอบ error จนกว่าจะตั้งค่า SDK
