# Routers

API route definitions and HTTP method handlers.

## 📋 Files

- **`router.go`** - Main router setup with all API endpoints and middleware

## Route Groups

### Public Routes (No Authentication)
- `POST /api/auth/register` - User registration
- `POST /api/auth/login` - User login
- `GET /api/devices/browse` - Browse all devices

### OAuth Routes
- `GET /api/auth/google` - Initiate Google login
- `GET /api/auth/google/callback` - Google OAuth callback

### Protected Routes (Requires Authentication)
- `GET /api/auth/profile` - Get user profile
- `GET /api/devices` - Admin: list all devices
- `POST /api/devices` - Create new device
- `GET /api/devices/:id` - Get device details
- `PUT /api/devices/:id` - Update device
- `DELETE /api/devices/:id` - Delete device
- `POST /api/devices/:id/review` - Post device review

## 🔒 Middleware Stack

1. **CORS Middleware** - Handle cross-origin requests
2. **Auth Middleware** - Verify JWT token
3. **KMITL Email Middleware** - Enforce @kmitl.ac.th email domain
4. **Request Validation** - Validate input data

Example middleware order for protected route:
```
Header → CORS → Auth → EmailCheck → Handler
```
