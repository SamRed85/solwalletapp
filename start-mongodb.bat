@echo off
echo Starting MongoDB with Docker...
echo.

REM Check if Docker is running
docker version >nul 2>&1
if %errorlevel% neq 0 (
    echo Error: Docker is not running or not installed.
    echo Please install Docker Desktop and start it.
    pause
    exit /b 1
)

REM Start MongoDB and Mongo Express
echo Starting MongoDB container...
docker-compose up -d

echo.
echo MongoDB is starting up...
echo.
echo Services:
echo - MongoDB: localhost:27017
echo - Mongo Express (Web UI): http://localhost:8081
echo.
echo Credentials:
echo - MongoDB Admin: admin / password123
echo - Mongo Express: admin / admin123
echo.
echo Waiting for MongoDB to be ready...
timeout /t 10 /nobreak >nul

echo.
echo MongoDB should be ready now!
echo You can now run your Go backend: go run main.go
echo.
pause
