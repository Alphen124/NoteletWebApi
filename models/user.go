package models

import (
	"database/sql"
	"time"
)

// AppUser โครงสร้างสำหรับ AppUser table
// ไม่ต้องมี Role เพราะ user ทุกคนเป็นทั้ง owner และ renter
type AppUser struct {
	UserId         int       `json:"user_id"`
	Email          string    `json:"email"`
	PasswordHash   string    `json:"-"` // ไม่ส่งใน JSON response
	IsActive       bool      `json:"is_active"`
	IsAdmin        bool      `json:"is_admin"`
	IsCentralStaff bool      `json:"is_central_staff"`
	CreatedAt      time.Time `json:"created_at"`
}

// Owner โครงสร้างสำหรับ Owner table
type Owner struct {
	OwnerNo int           `json:"owner_no"`
	Name    string        `json:"name"`
	FName   string        `json:"fname"`
	LName   string        `json:"lname"`
	Tel     string        `json:"tel"`
	Rating  int           `json:"rating"`
	UserId  sql.NullInt64 `json:"user_id,omitempty"`
}

// Renter โครงสร้างสำหรับ Renter table
type Renter struct {
	RenterNo int           `json:"renter_no"`
	Name     string        `json:"name"`
	FName    string        `json:"fname"`
	LName    string        `json:"lname"`
	Tel      string        `json:"tel"`
	Rating   int           `json:"rating"`
	UserId   sql.NullInt64 `json:"user_id,omitempty"`
}
