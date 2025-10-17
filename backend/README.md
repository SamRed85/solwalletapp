# Sol Backend API

A Go-based REST API with MongoDB integration for user management.

## Features

- 🚀 RESTful API endpoints
- 🗄️ MongoDB integration for persistent storage
- 🔄 CORS support for frontend communication
- 📊 Health check endpoint
- 👥 User CRUD operations

## Prerequisites

- Go 1.21 or higher
- MongoDB (running via Docker)

## Setup

1. **Install dependencies:**
   ```bash
   cd backend
   go mod tidy
   ```

2. **Start MongoDB (from project root):**
   ```bash
   docker-compose up -d
   ```

3. **Run the backend:**
   ```bash
   go run main.go
   ```

The server will start on `http://localhost:8080`

## API Endpoints

### Health Check
- **GET** `/health` - Check server and database health

### Users
- **GET** `/api/users` - Get all users
- **GET** `/api/users/{id}` - Get user by ID
- **POST** `/api/users` - Create new user

### Example Usage

**Create a user:**
```bash
curl -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{"name": "John Doe", "email": "john@example.com"}'
```

**Get all users:**
```bash
curl http://localhost:8080/api/users
```

**Health check:**
```bash
curl http://localhost:8080/health
```

## Environment Variables

- `MONGODB_URI` - MongoDB connection string (default: `mongodb://localhost:27017`)

## Database

The API uses MongoDB with the following structure:
- Database: `sol_db`
- Collection: `users`

User document structure:
```json
{
  "_id": "ObjectId",
  "name": "string",
  "email": "string"
}
```
