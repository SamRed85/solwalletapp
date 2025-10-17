# Sol - Full Stack Application

A modern full-stack application with Vue.js frontend, Go backend, and MongoDB database.

## 🏗️ Project Structure

```
Sol/
├── backend/                 # Go REST API
│   ├── main.go             # Main server file
│   ├── go.mod              # Go dependencies
│   ├── go.sum              # Go dependencies lock
│   └── README.md           # Backend documentation
├── frontend/               # Vue.js frontend
│   ├── src/                # Source code
│   ├── public/             # Static assets
│   ├── package.json        # Node dependencies
│   ├── vue.config.js       # Vue CLI configuration
│   └── README.md           # Frontend documentation
├── docker-compose.yml      # MongoDB and Mongo Express
├── mongo-init.js          # MongoDB initialization script
└── README.md              # This file
```

## 🚀 Quick Start

### Prerequisites
- Go 1.21+
- Node.js 14+
- Docker & Docker Compose

### 1. Start MongoDB
```bash
docker-compose up -d
```

### 2. Start Backend
```bash
cd backend
go mod tidy
go run main.go
```
Backend runs on: `http://localhost:8080`

### 3. Start Frontend
```bash
cd frontend
npm install
npm run serve
```
Frontend runs on: `http://localhost:3000`

## 🎯 Features

### Backend (Go + MongoDB)
- ✅ RESTful API with CORS support
- ✅ MongoDB integration for persistent storage
- ✅ Health check endpoint
- ✅ User CRUD operations
- ✅ Docker-ready configuration

### Frontend (Vue.js + Vuetify 2)
- ✅ Beautiful Material Design UI
- ✅ Responsive navigation and layout
- ✅ User management with data tables
- ✅ Real-time backend health monitoring
- ✅ Form validation and error handling

### Database (MongoDB)
- ✅ Persistent data storage
- ✅ Web interface via Mongo Express
- ✅ Automatic database initialization

## 📊 API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/` | Home page with API info |
| GET | `/health` | Health check |
| GET | `/api/users` | Get all users |
| GET | `/api/users/{id}` | Get user by ID |
| POST | `/api/users` | Create new user |

## 🗄️ Database

- **Database**: `sol_db`
- **Collection**: `users`
- **Web Interface**: `http://localhost:8081` (admin/admin123)

## 🛠️ Development

### Backend Development
```bash
cd backend
go run main.go
```

### Frontend Development
```bash
cd frontend
npm run serve
```

### Database Management
```bash
# Start MongoDB
docker-compose up -d

# Stop MongoDB
docker-compose down

# View logs
docker-compose logs -f mongodb
```

## 📁 Project Organization

This project follows a clean separation of concerns:

- **`backend/`**: Contains all Go server code, dependencies, and backend-specific documentation
- **`frontend/`**: Contains all Vue.js application code, dependencies, and frontend-specific documentation
- **Root level**: Contains shared infrastructure (Docker, database initialization, and project overview)

## 🔧 Configuration

### Environment Variables
- `MONGODB_URI`: MongoDB connection string (default: `mongodb://localhost:27017`)

### Ports
- Backend: `8080`
- Frontend: `3000`
- MongoDB: `27017`
- Mongo Express: `8081`

## 📚 Documentation

- [Backend Documentation](./backend/README.md)
- [Frontend Documentation](./frontend/README.md)
- [MongoDB Setup Guide](./MONGODB_SETUP.md)

## 🎨 Tech Stack

- **Backend**: Go, MongoDB, Docker
- **Frontend**: Vue.js 2, Vuetify 2, Vue Router, Axios
- **Database**: MongoDB 7.0
- **Infrastructure**: Docker, Docker Compose

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test thoroughly
5. Submit a pull request

## 📄 License

This project is licensed under the MIT License.