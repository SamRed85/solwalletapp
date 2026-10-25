@echo off
echo Starting Sol (MongoDB, backend, frontend)...

if exist "%~dp0.env" (
  for /f "usebackq eol=# tokens=1,* delims==" %%A in ("%~dp0.env") do (
    if /I "%%A"=="SOLANA_RPC_URL" set "SOLANA_RPC_URL=%%B"
  )
)

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
