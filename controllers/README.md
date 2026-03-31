# controllers/

HTTP request handlers สำหรับทุก API endpoint ในระบบ  
แต่ละ controller รับ `*sql.DB` ผ่าน dependency injection จาก `main.go`

## 📋 โครงสร้างไฟล์

```
controllers/
├── auth.go            # Authentication (register, login, token refresh, profile)
├── device.go          # Device CRUD + status management
├── oauth.go           # Google OAuth2
├── firebase_auth.go   # Firebase ID token auth
├── supabase_auth.go   # Supabase access token auth
├── rental_request.go  # Rental request lifecycle
├── review.go          # Device reviews + user-to-user reviews
├── chat.go            # WebSocket real-time chat + notifications
└── upload.go          # Image file upload
```

---

## auth.go — `AuthController`

| Method | Path | Auth | หน้าที่ |
|---|---|---|---|
| POST | `/api/auth/register` | ไม่ต้อง | สมัครสมาชิก (เฉพาะ @kmitl.ac.th) สร้าง Owner + Renter อัตโนมัติ |
| POST | `/api/auth/login` | ไม่ต้อง | เข้าสู่ระบบ คืน AccessToken + RefreshToken |
| POST | `/api/auth/refresh` | ไม่ต้อง | ขอ AccessToken ใหม่ด้วย RefreshToken |
| GET | `/api/auth/profile` | ✅ JWT | ดูโปรไฟล์ตนเอง (Owner + Renter info) |
| POST | `/api/admin/register` | X-Admin-Secret header | สร้าง admin user (ข้ามข้อจำกัด email domain) |

**Register Flow:**
1. ตรวจสอบ email domain (@kmitl.ac.th หรือ whitelist)
2. hash password ด้วย bcrypt
3. Transaction: สร้าง AppUser → Owner → Renter พร้อมกัน
4. คืน JWT token pair

---

## device.go — `DeviceController`

| Method | Path | Auth | หน้าที่ |
|---|---|---|---|
| GET | `/api/devices/browse` | ไม่ต้อง | ดูอุปกรณ์ที่ว่าง (Status=1), filter `?type=Notebook\|MacBook\|Other` |
| POST | `/api/devices` | ✅ JWT | เพิ่มอุปกรณ์ใหม่ |
| GET | `/api/devices` | ✅ JWT | ดูอุปกรณ์ทั้งหมด (admin view) |
| GET | `/api/devices/my` | ✅ JWT | ดูอุปกรณ์ของตนเอง |
| GET | `/api/devices/{id}` | ไม่ต้อง | ดูรายละเอียดอุปกรณ์ชิ้นเดียว |
| PUT | `/api/devices/{id}` | ✅ JWT (เจ้าของ/admin) | แก้ไขอุปกรณ์ (ห้ามขณะเช่า) |
| DELETE | `/api/devices/{id}` | ✅ JWT (เจ้าของ/admin) | ลบอุปกรณ์ |
| PATCH | `/api/devices/{id}/status` | ✅ JWT (เจ้าของ) | เปลี่ยนสถานะอุปกรณ์ |
| GET | `/api/devices/{id}/history` | ✅ JWT | ดูประวัติสถานะอุปกรณ์ |

**Device Status:**
```
1 = Available → 2 = Booking Confirmed → 3 = Rental Active → 4 = Rental Completed
```

---

## oauth.go — `OAuthController`

| Method | Path | Auth | หน้าที่ |
|---|---|---|---|
| POST | `/api/auth/google` | ไม่ต้อง | เริ่ม Google OAuth flow (redirect ไป Google) |
| GET | `/api/auth/google/callback` | ไม่ต้อง | รับ code จาก Google, แลก token, สร้าง/ดึง user, คืน JWT |

---

## firebase_auth.go — `FirebaseAuthController`

| Method | Path | Auth | หน้าที่ |
|---|---|---|---|
| POST | `/api/auth/firebase` | ไม่ต้อง | รับ Firebase ID token → ยืนยันกับ Firebase Admin SDK → คืน app JWT |

**Request Body:** `{ "id_token": "..." }`

---

## supabase_auth.go — `SupabaseAuthController`

| Method | Path | Auth | หน้าที่ |
|---|---|---|---|
| POST | `/api/auth/supabase` | ไม่ต้อง | รับ Supabase access token → ตรวจสอบกับ Supabase API → คืน app JWT |

**Request Body:** `{ "access_token": "..." }`

---

## rental_request.go — `RentalController`

| Method | Path | Auth | หน้าที่ |
|---|---|---|---|
| POST | `/api/rental-requests` | ✅ JWT (renter) | ส่งคำขอเช่า |
| GET | `/api/rental-requests/{id}` | ✅ JWT | ดูรายละเอียดคำขอ |
| GET | `/api/rental-requests/incoming` | ✅ JWT (เจ้าของ) | ดูคำขอที่เข้ามา |
| GET | `/api/rental-requests/outgoing` | ✅ JWT (ผู้เช่า) | ดูคำขอที่ส่งออก |
| PATCH | `/api/rental-requests/{id}/confirm` | ✅ JWT (เจ้าของ) | อนุมัติ (สร้าง Schedule + RentBill + Reservation) |
| PATCH | `/api/rental-requests/{id}/reject` | ✅ JWT (เจ้าของ) | ปฏิเสธ |
| PATCH | `/api/rental-requests/{id}/active` | ✅ JWT (เจ้าของ) | เริ่มการเช่าจริง |
| PATCH | `/api/rental-requests/{id}/returned` | ✅ JWT (เจ้าของ) | ยืนยันคืนอุปกรณ์ → Device กลับเป็น Available |
| PATCH | `/api/rental-requests/{id}/cancel` | ✅ JWT (ผู้เช่า) | ยกเลิกคำขอ |
| PATCH | `/api/rental-requests/{id}/update-dates` | ✅ JWT (ผู้เช่า) | แก้วันที่ (เฉพาะ status = Request Pending) |
| POST | `/api/chat/confirm-rental` | ✅ JWT (เจ้าของ) | ยืนยันการเช่าจากแชท (ข้ามขั้นตอน request flow) |

---

## review.go — `ReviewController`

| Method | Path | Auth | หน้าที่ |
|---|---|---|---|
| GET | `/api/devices/{id}/reviews` | ไม่ต้อง | ดูรีวิวอุปกรณ์ + คะแนนเฉลี่ย |
| POST | `/api/devices/{id}/reviews` | ✅ JWT | เพิ่ม/แก้ไขรีวิวอุปกรณ์ (rating 1–5, upsert) |
| POST | `/api/reviews` | ✅ JWT | รีวิว user (renter ↔ owner) ตาม rental request |
| GET | `/api/reviews/eligibility?requestNo=X` | ✅ JWT | ตรวจว่ายังสามารถรีวิวได้หรือไม่ |
| PATCH | `/api/reviews/{reviewNo}/reply` | ✅ JWT (ผู้ถูกรีวิว) | ตอบกลับรีวิว (ตอบได้ครั้งเดียว) |
| GET | `/api/users/{userId}/reviews` | ไม่ต้อง | ดูรีวิวที่ user ได้รับ (แยก role owner/renter) |
| GET | `/api/users/{userId}/rating` | ไม่ต้อง | ดูคะแนนเฉลี่ยในฐานะ owner และ renter |

---

## chat.go — `ChatController`

| Method | Path | Auth | หน้าที่ |
|---|---|---|---|
| GET (WS) | `/api/chat/ws?token=&room=` | JWT ใน query | WebSocket: real-time chat, typing, history, notifications |
| GET | `/api/chat/rooms` | ✅ JWT | ดูรายการ chat rooms |
| POST | `/api/chat/device-room` | ✅ JWT | สร้าง/ดึง room สำหรับเจรจาเช่า |
| POST | `/api/chat/upload-image` | ✅ JWT | อัปโหลดรูปภาพในแชท |
| GET | `/api/chat/notifications` | ✅ JWT | ดู notifications ทั้งหมด |
| GET | `/api/chat/notifications/unread` | ✅ JWT | นับ unread count |
| PATCH | `/api/chat/notifications/read` | ✅ JWT | mark as read |
| GET | `/api/chat/owner-rooms` | ✅ JWT | ดู rooms ของเจ้าของพร้อม last message preview |

**WebSocket Message Types:** `chat`, `typing`, `history`, `online`, `notification`, `join`, `leave`

---

## upload.go — `UploadController`

| Method | Path | Auth | หน้าที่ |
|---|---|---|---|
| POST | `/api/upload/images` | ✅ JWT | อัปโหลดรูปภาพ (max 5 ไฟล์, 10MB/ไฟล์) |

**Allowed types:** `.jpg`, `.jpeg`, `.png`, `.gif`, `.webp`  
**Storage:** `./uploads/` directory

---

## Response Format

**Success:**
```json
{
  "success": true,
  "message": "...",
  "data": { ... }
}
```

**Error:**
```json
{
  "success": false,
  "message": "..."
}
```
