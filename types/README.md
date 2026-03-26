# Types

Type definitions for API requests and responses.

## 📋 Subdirectories

- **`requests/`** - Input DTOs (Data Transfer Objects)
  - `auth_request.go` - Login, register, token refresh
  - `device_request.go` - Device CRUD operations
  
- **`responses/`** - Output DTOs
  - `response.go` - Standard response wrapper

## Purpose

Separating request/response types from database models:
- ✅ Validation and constraints on input
- ✅ Selective field exposure (no passwords in responses)
- ✅ Version compatibility for API changes
- ✅ Clear API contract documentation

## Example

```go
// Request Type
type LoginRequest struct {
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=6"`
}

// Database Model
type User struct {
    ID           int
    Email        string
    PasswordHash string  // Never exposed in response
}

// Response Type
type UserResponse struct {
    ID    int    `json:"id"`
    Email string `json:"email"`
}
```
