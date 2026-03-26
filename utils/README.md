# Utils

Utility functions for common operations.

## 📋 Files

- **`password.go`** - Password hashing and validation using bcrypt
- **`email.go`** - Email sending and validation

## Functions

### Password Utils
- `HashPassword(password string) string` - Create bcrypt hash
- `VerifyPassword(hash, password string) bool` - Verify password against hash

### Email Utils
- `SendEmail(to, subject, body string) error` - Send email via SMTP
- `IsValidEmail(email string) bool` - Validate email format
