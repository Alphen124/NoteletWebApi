# utils/

ฟังก์ชัน utility ที่ใช้แชร์ร่วมทั่วโปรเจกต์

## 📋 โครงสร้างไฟล์

```
utils/
├── password.go    # bcrypt password hashing
└── email.go       # ตรวจสอบ email domain
```

---

## password.go

### ฟังก์ชัน

#### `HashPassword(password string) (string, error)`
สร้าง bcrypt hash จาก password plaintext
- ใช้ `bcrypt.DefaultCost` (cost = 10)
- เรียกใน: `Register`, `AdminRegister`, และ seed admin ใน `main.go`

```go
hashedPassword, err := utils.HashPassword(req.Password)
```

#### `CheckPasswordHash(password, hash string) bool`
เปรียบ password plaintext กับ bcrypt hash
- คืน `true` ถ้า password ตรงกัน
- เรียกใน: `Login`

```go
if !utils.CheckPasswordHash(req.Password, user.PasswordHash) {
    // รหัสผ่านไม่ถูกต้อง
}
```

---

## email.go

### ฟังก์ชัน

#### `IsKMITLEmail(email string) bool` (ใช้งานอยู่)
ตรวจสอบว่าอีเมลลงท้ายด้วย `@kmitl.ac.th` หรือไม่
- trim และ lowercase ก่อนตรวจ
- เรียกใน: หลาย controllers + middleware

```go
// ตัวอย่างการใช้งาน
if !utils.IsKMITLEmail(req.Email) {
    // ปฏิเสธ: ไม่ใช่ KMITL email
}
```

#### `ValidateEmail(email string) bool` (ไม่ได้ใช้งาน — dead code)
ตรวจสอบรูปแบบอีเมลเบื้องต้น (มี `@` และยาวกว่า 3 ตัวอักษร)  
> ⚠️ ปัจจุบันไม่มีที่ใดใช้งานฟังก์ชันนี้ในโค้ด
