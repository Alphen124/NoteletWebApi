# Notelet Backend Service

Go-based REST API backend for the Notelet device rental system.

## 📂 Directory Structure

- **`config/`** - Configuration files (database connection, environment setup)
- **`controllers/`** - HTTP request handlers for business logic
- **`models/`** - Database models and data structures
- **`routers/`** - API route definitions and middleware setup
- **`middlewares/`** - Custom middleware (authentication, CORS, validation)
- **`services/`** - Business logic and external services (OAuth, JWT, email)
- **`types/`** - Type definitions for requests and responses
- **`utils/`** - Utility functions (password hashing, email sending)
- **`migrations/`** - Database schema changes and migrations
- **`uploads/`** - Storage directory for uploaded files

## 🚀 Getting Started

1. **Install Go** (1.18+)
2. **Set up environment**:
   ```bash
   cp .env.example .env
   # Edit .env with your configuration
   ```
3. **Install dependencies**:
   ```bash
   go mod download
   ```
4. **Run server**:
   ```bash
   go run main.go
   ```
   Server will start on `http://localhost:3001`

## 🗄️ Database

PostgreSQL is required. Run migrations at startup:
```bash
# Migrations are executed automatically on startup
# Check migrations/ folder for SQL scripts
```

## 📡 API Endpoints

See [API_DEVICE_DOCUMENTATION.md](./API_DEVICE_DOCUMENTATION.md) for detailed API documentation.

## 🔐 Authentication

- **Google OAuth**: `/api/auth/google` → `/api/auth/google/callback`
- **JWT**: Issue and refresh tokens for API requests
- **KMITL Email**: Enforce email domain restrictions

## 🛠️ Development

### Hot Reload
Use `air` for development:
```bash
go install github.com/cosmtrek/air@latest
air
```

### Build
```bash
go build -o notelet-api
./notelet-api
```

### Docker
```bash
docker-compose up
```
