# Services

Business logic layer that handles complex operations and external service integrations.

## 📋 Subdirectories

- **`jwt/`** - JWT token generation and validation
- **`oauth/`** - OAuth 2.0 integration (Google)
- **`email.go`** - Email sending utilities
- **`password.go`** - Password hashing and validation

## Purpose

Services encapsulate:
- Database operations
- External API calls (OAuth, email)
- Cryptographic operations
- Business logic

This separation makes code testable and reusable.

## Example Usage

```go
// JWT Service
token, err := jwt.GenerateToken(userID, email)

// OAuth Service
user, err := oauth.GetGoogleUserInfo(accessToken)

// Password Service
hash := password.HashPassword(plaintext)
```
