@echo off
echo Starting Sol (MongoDB, backend, frontend)...

docker compose up -d
if %errorlevel% neq 0 (
    echo Docker is not running. Start Docker Desktop and try again.
    pause
    exit /b 1
)

start "Sol backend" cmd /k "cd backend && go run ."
start "Sol frontend" cmd /k "cd frontend && npm install && npm run dev"

echo.
echo Backend:  http://localhost:8080
echo Frontend: http://localhost:3000
