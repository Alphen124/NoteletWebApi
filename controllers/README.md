# Controllers

HTTP request handlers that process client requests and return responses.

## 📋 Files

- **`auth.go`** - Authentication endpoints (register, login, refresh token, profile)
- **`device.go`** - Device CRUD operations and browsing
- **`oauth.go`** - OAuth 2.0 authentication (Google login/callback)
- **`upload.go`** - File upload handling
- **`rental_request.go`** - Rental request management
- **`review.go`** - Device review operations

## 🏗️ Architecture Pattern

Each controller follows this pattern:
1. Parse request data
2. Validate input
3. Call service layer
4. Return formatted response

Example:
```go
func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request
    // 2. Validate
    // 3. Call service
    // 4. Return response
}
```

## 📤 Response Format

All responses follow this standard format:
```json
{
    "success": true,
    "status": 200,
    "data": {...},
    "message": "Success message"
}
```

Error responses:
```json
{
    "success": false,
    "status": 400,
    "message": "Error description"
}
```
