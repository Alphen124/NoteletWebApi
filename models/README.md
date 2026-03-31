# models/

Go structs สำหรับ map กับตาราง PostgreSQL

## 📋 โครงสร้างไฟล์

```
models/
├── user.go      # AppUser, Owner, Renter
└── device.go    # Device, DeviceType
```

> **หมายเหตุ:** Controllers ส่วนใหญ่ใช้ inline struct แทนการ import models  
> ปัจจุบัน `models` package ถูก import โดย: `auth.go`, `oauth.go`, `firebase_auth.go`, `supabase_auth.go`

---

## user.go

### `AppUser`
ตรงกับตาราง `appuser` ในฐานข้อมูล
```go
type AppUser struct {
    UserId       int       `json:"user_id"`
    Email        string    `json:"email"`
    PasswordHash string    `json:"-"`        // ไม่ส่งใน response
    IsActive     bool      `json:"is_active"`
    IsAdmin      bool      `json:"is_admin"`
    CreatedAt    time.Time `json:"created_at"`
}
```

### `Owner`
ตรงกับตาราง `owner`
```go
type Owner struct {
    OwnerNo int           `json:"owner_no"`
    Name    string        `json:"name"`
    FName   string        `json:"fname"`
    LName   string        `json:"lname"`
    Tel     string        `json:"tel"`
    Rating  int           `json:"rating"`
    UserId  sql.NullInt64 `json:"user_id,omitempty"`
}
```
> ⚠️ ปัจจุบัน `Owner` struct ไม่ได้ถูกใช้งาน — controllers ใช้ SQL query + scan ตรงแทน

### `Renter`
ตรงกับตาราง `renter`
```go
type Renter struct {
    RenterNo int           `json:"renter_no"`
    Name     string        `json:"name"`
    FName    string        `json:"fname"`
    LName    string        `json:"lname"`
    Tel      string        `json:"tel"`
    Rating   int           `json:"rating"`
    UserId   sql.NullInt64 `json:"user_id,omitempty"`
}
```
> ⚠️ ปัจจุบัน `Renter` struct ไม่ได้ถูกใช้งาน — controllers ใช้ SQL query + scan ตรงแทน

---

## device.go

### `Device`
ตรงกับตาราง `device`
```go
type Device struct {
    DeviceNo     int       `json:"deviceNo"`
    DeviceName   string    `json:"deviceName"`
    Description  string    `json:"description"`
    RentPrice    float64   `json:"rentPrice"`
    TypeNo       *int      `json:"typeNo,omitempty"`
    DeviceTypeNo *int      `json:"deviceTypeNo,omitempty"`
    Rating       float64   `json:"rating"`
    UserId       int       `json:"userId"`
    Status       string    `json:"status"`
    Condition    string    `json:"condition,omitempty"`
    ImageUrl     string    `json:"imageUrl,omitempty"`
    CreatedAt    time.Time `json:"createdAt"`
}
```
> ⚠️ ปัจจุบัน `Device` struct ไม่ได้ถูกใช้งาน — `device.go` controller ใช้ inline struct แทน

### `DeviceType`
```go
type DeviceType struct {
    DeviceTypeNo   int    `json:"deviceTypeNo"`
    DeviceTypeName string `json:"deviceTypeName"`
}
```

---

## ตาราง Database ที่เกี่ยวข้อง

| ตาราง | ความสัมพันธ์ |
|---|---|
| `appuser` | ผู้ใช้หลัก (1 user = 1 owner + 1 renter) |
| `owner` | FK → appuser.userid |
| `renter` | FK → appuser.userid |
| `device` | FK → owner.ownerno |
| `devicetype` | ประเภทอุปกรณ์ (Notebook, MacBook, Other) |
| `status` | สถานะ (1=Available, 2=Booking Confirmed, 3=Rental Active, 4=Rental Completed) |
| `devicestatushistory` | ประวัติการเปลี่ยนสถานะ |
| `rentalrequest` | คำขอเช่า |
| `schedule` | ตาราง rental schedule |
| `rentbill` | ใบเสร็จการเช่า |
| `review` | รีวิวอุปกรณ์ |
| `userreview` | รีวิว user-to-user |
| `chatroom` | ห้องแชท |
| `chatmessage` | ข้อความในแชท |
| `chatnotification` | การแจ้งเตือนแชท |
