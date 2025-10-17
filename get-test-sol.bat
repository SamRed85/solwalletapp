@echo off
echo 🚀 Getting Test SOL for Solana Development
echo ==========================================

REM Check if Solana CLI is installed
solana --version >nul 2>&1
if %errorlevel% neq 0 (
    echo ❌ Solana CLI not found. Please install it first.
    echo.
    echo 📥 Install Solana CLI:
    echo 1. Download from: https://github.com/solana-labs/solana/releases
    echo 2. Or use: winget install Solana.SolanaCLI
    echo 3. Restart your terminal after installation
    echo.
    pause
    exit /b 1
)

echo ✅ Solana CLI found!

REM Set to devnet
echo 🔧 Setting network to devnet...
solana config set --url devnet

REM Show current configuration
echo 📋 Current configuration:
solana config get

echo.
echo 💰 Getting test SOL...

REM Get wallet address
for /f "tokens=*" %%i in ('solana address') do set WALLET_ADDRESS=%%i
echo 📍 Wallet address: %WALLET_ADDRESS%

REM Airdrop SOL
echo 🪂 Requesting airdrop...
solana airdrop 2

REM Check balance
echo 💳 Current balance:
solana balance

echo.
echo ✅ Done! You now have test SOL for development.
echo.
echo 🎯 Next steps:
echo 1. Make sure Phantom wallet is set to 'Devnet' network
echo 2. Go to your app: http://localhost:3000/auth
echo 3. Connect your wallet and authenticate!
echo.
echo 💡 Pro tip: You can run this script multiple times to get more SOL
echo.
pause
