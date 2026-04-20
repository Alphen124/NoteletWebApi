package requests

// RegisterRequest โครงสร้างสำหรับการลงทะเบียน
// ไม่ต้องระบุ role เพราะจะสร้างทั้ง owner และ renter พร้อมกัน
type RegisterRequest struct {
	Email          string `json:"email" validate:"required,email"`
	Password       string `json:"password" validate:"required,min=6"`
	FName          string `json:"fname" validate:"required"`
	LName          string `json:"lname" validate:"required"`
	Tel            string `json:"tel" validate:"required"`
	IsCentralStaff bool   `json:"is_central_staff"`
}

// LoginRequest โครงสร้างสำหรับการเข้าสู่ระบบ
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// RefreshTokenRequest โครงสร้างสำหรับการขอ refresh token
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}
