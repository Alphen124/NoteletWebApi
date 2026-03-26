# Middlewares

HTTP middleware functions that process requests before reaching handlers.

## 📋 Files

- **`auth.go`** - JWT authentication and authorization middleware

## Middleware Functions

- `AuthMiddleware` - Validates JWT token from Authorization header
- `CORSMiddleware` - Handles Cross-Origin Resource Sharing
- `KMITLEmailMiddleware` - Restricts access to @kmitl.ac.th email domain

## 🔒 How Authentication Works

1. Client sends JWT token in `Authorization: Bearer <token>` header
2. Middleware extracts and validates token
3. If valid, extracts user ID and adds to request context
4. Handler accesses user ID from context

```go
userID := r.Context().Value("userID").(string)
```

## Token Format

JWT tokens contain:
- User ID
- Email
- Expiration time
- Signature (HS256)
