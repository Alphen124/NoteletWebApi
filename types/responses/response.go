package responses

// SuccessResponse โครงสร้างสำหรับ response ที่สำเร็จ
type SuccessResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ErrorResponse โครงสร้างสำหรับ response ที่เกิด error
type ErrorResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// AuthResponse โครงสร้างสำหรับ response ของการ authentication
type AuthResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	User         interface{} `json:"user"`
}

// UserResponse โครงสร้างสำหรับข้อมูล user ที่ส่งกลับ
type UserResponse struct {
	UserId   int    `json:"user_id"`
	Email    string `json:"email"`
	IsActive bool   `json:"is_active"`
	FName    string `json:"fname,omitempty"`
	LName    string `json:"lname,omitempty"`
	Tel      string `json:"tel,omitempty"`
}

// DualRoleUserResponse โครงสร้างสำหรับข้อมูล user ที่เป็นทั้ง owner และ renter
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
