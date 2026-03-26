# Configuration

Handles application configuration and database setup.

## 📋 Files

- **`database/database.go`** - PostgreSQL connection configuration and initialization

## 🔧 Environment Files

- `.env` - Local environment variables (add to .gitignore)
- `.env.example` - Template for required environment variables

## Required Environment Variables

```
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=notelet
GOOGLE_CLIENT_ID=your_client_id
GOOGLE_CLIENT_SECRET=your_secret
GOOGLE_REDIRECT_URL=http://localhost:3001/api/auth/google/callback
```
