# middlewares/

HTTP middleware functions ที่ทำงานก่อน handler จะถูกเรียก

## 📋 โครงสร้างไฟล์

```
middlewares/
└── auth.go    # AuthMiddleware, KMITLEmailMiddleware, CORSMiddleware
```

---

## auth.go

### Middleware Functions

#### 1. `AuthMiddleware(next http.Handler) http.Handler`
ตรวจสอบ JWT access token จาก `Authorization` header

**ขั้นตอน:**
1. ตรวจสอบว่ามี `Authorization` header
2. ตรวจรูปแบบ `Bearer <token>`
3. ถอดรหัสและ validate token ด้วย `jwt.ValidateAccessToken()`
4. ฉีด `UserContext` เข้าไปใน request context

**เมื่อ token ไม่ถูกต้อง:** คืน `401 Unauthorized`

```go
// ดึงข้อมูล user ใน handler
userCtx, ok := middlewares.GetUserFromContext(r)
// userCtx.UserId, userCtx.Email, userCtx.IsAdmin
```

#### 2. `KMITLEmailMiddleware(next http.Handler) http.Handler`
บังคับให้ user ต้องใช้อีเมล `@kmitl.ac.th` เท่านั้น

- ต้องใช้ **หลัง** `AuthMiddleware` เสมอ (ต้องมี UserContext)
- Admin (`IsAdmin=true`) ไม่ผ่าน middleware นี้ เพราะ route admin แยกต่างหาก
- เมื่อ email ไม่ใช่ @kmitl.ac.th: คืน `403 Forbidden`

> ปัจจุบันใช้กับ `GET /api/auth/profile` เท่านั้น

#### 3. `CORSMiddleware(next http.Handler) http.Handler`
อนุญาต Cross-Origin requests จากทุก origin

**Headers ที่เพิ่ม:**
```
Access-Control-Allow-Origin:  *
Access-Control-Allow-Methods: GET, POST, PUT, DELETE, PATCH, OPTIONS
Access-Control-Allow-Headers: Content-Type, Authorization
```
- `OPTIONS` preflight request → ตอบ `200 OK` ทันที

### UserContext Struct

```go
type UserContext struct {
    UserId  int
    Email   string
    IsAdmin bool
}
```
เก็บใน request context ด้วย key `UserContextKey` (type: `ContextKey`)

### Helper Functions

| ฟังก์ชัน | หน้าที่ |
|---|---|
| `GetUserFromContext(r *http.Request)` | ดึง `*UserContext` จาก context, คืน `(ctx, ok)` |
| `respondWithError(w, code, message)` | ส่ง JSON error response |

---

## Middleware Stack ของแต่ละ Route Group

```
Public routes:     request → Handler
Protected routes:  request → CORSMiddleware → AuthMiddleware → Handler
Profile route:     request → CORSMiddleware → AuthMiddleware → KMITLEmailMiddleware → Handler
```
