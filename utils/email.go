package utils

import "strings"

// IsKMITLEmail ตรวจสอบว่าอีเมลเป็น @kmitl.ac.th หรือไม่
func IsKMITLEmail(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	return strings.HasSuffix(email, "@kmitl.ac.th")
}

// ValidateEmail ตรวจสอบรูปแบบอีเมล (เบื้องต้น)
func ValidateEmail(email string) bool {
	email = strings.TrimSpace(email)
	return strings.Contains(email, "@") && len(email) > 3
}
