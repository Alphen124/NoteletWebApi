# config/

โฟลเดอร์นี้จัดการการเชื่อมต่อฐานข้อมูลและการตั้งค่า environment ของแอปพลิเคชัน

## 📋 โครงสร้างไฟล์

```
config/
└── database/
    └── database.go    # PostgreSQL singleton connection
```

---

## database/database.go

### หน้าที่
- สร้าง connection กับ PostgreSQL แบบ **Singleton** (เปิดครั้งเดียว ใช้ทั้ง app)
- รองรับ 2 รูปแบบการตั้งค่า:
  1. `DATABASE_URL` — สำหรับ Railway / cloud providers
  2. ตัวแปรแยก (`DB_HOST`, `DB_PORT`, ...) — สำหรับ local development

### ฟังก์ชัน

| ฟังก์ชัน | คืนค่า | หน้าที่ |
|---|---|---|
| `ConnectNoteletDB()` | `*sql.DB` | เชื่อมต่อ PostgreSQL และคืน instance |

### Logic การเชื่อมต่อ
```
มี DATABASE_URL?
  ├── ใช่ → parse URL, บังคับ sslmode=require (ถ้ายังไม่มี)
  └── ไม่ใช่ → อ่าน DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
              → สร้าง connection string แบบ sslmode=disable (local)
```

### Environment Variables ที่ต้องการ

#### สำหรับ Production (Railway)
```env
DATABASE_URL=postgres://user:password@host:5432/notelet?sslmode=require
```

#### สำหรับ Local Development
```env
DB_HOST=localhost        # default: localhost
DB_PORT=5432             # default: 5432
DB_USER=alphen           # default: alphen
DB_PASSWORD=your_password  # จำเป็นต้องระบุ (ไม่มี default)
DB_NAME=notelet          # default: notelet
```

> **หมายเหตุ:** `DB_PASSWORD` หรือ `DATABASE_URL` อย่างใดอย่างหนึ่งจำเป็นต้องมี ไม่เช่นนั้น app จะ crash ทันที

### Environment Variables อื่นๆ ที่ใช้ในโปรเจกต์

```env
# JWT
JWT_ACCESS_SECRET=...      # จำเป็น — secret สำหรับ sign access token (หมดอายุ 24h)
JWT_REFRESH_SECRET=...     # จำเป็น — secret สำหรับ sign refresh token (หมดอายุ 7d)

# Google OAuth
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
GOOGLE_REDIRECT_URL=http://localhost:3001/api/auth/google/callback

# Admin
ADMIN_SECRET=notelet-admin-secret-2026   # ใช้ใน POST /api/admin/register

# Firebase (optional)
# วาง serviceAccountKey.json ไว้ที่ root หรือตั้ง GOOGLE_APPLICATION_CREDENTIALS
```

### ตัวอย่างการใช้งาน
```go
import database "noteletwebservice-development/config/database"

db := database.ConnectNoteletDB()
defer db.Close()
// ส่ง db ไปยัง controllers ทุกตัว
```
