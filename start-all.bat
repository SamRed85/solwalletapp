@echo off
echo Starting Sol Full Stack Application...
echo.

echo Starting MongoDB...
start "MongoDB" cmd /k "docker-compose up"

echo Waiting for MongoDB to start...
timeout /t 5 /nobreak > nul

echo Starting Backend...
start "Backend" cmd /k "cd backend && go run main.go"

echo Waiting for Backend to start...
timeout /t 3 /nobreak > nul

echo Starting Frontend...
start "Frontend" cmd /k "cd frontend && npm run serve"

echo.
echo All services started!
echo - MongoDB: http://localhost:27017
echo - Backend: http://localhost:8080
echo - Frontend: http://localhost:3000
echo - Mongo Express: http://localhost:8081
echo.
pause
