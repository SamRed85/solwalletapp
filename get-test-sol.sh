#!/bin/bash

# Script to get test SOL for development
# This is the most reliable method to get test SOL

echo "🚀 Getting Test SOL for Solana Development"
echo "=========================================="

# Check if Solana CLI is installed
if ! command -v solana &> /dev/null; then
    echo "❌ Solana CLI not found. Installing..."
    
    # Install Solana CLI
    sh -c "$(curl -sSfL https://release.solana.com/v1.17.0/install)"
    
    # Add to PATH
    export PATH="$HOME/.local/share/solana/install/active_release/bin:$PATH"
    
    echo "✅ Solana CLI installed!"
    echo "⚠️  Please restart your terminal or run: export PATH=\"\$HOME/.local/share/solana/install/active_release/bin:\$PATH\""
    echo ""
fi

# Set to devnet
echo "🔧 Setting network to devnet..."
solana config set --url devnet

# Show current configuration
echo "📋 Current configuration:"
solana config get

echo ""
echo "💰 Getting test SOL..."

# Get wallet address
WALLET_ADDRESS=$(solana address)
echo "📍 Wallet address: $WALLET_ADDRESS"

# Airdrop SOL
echo "🪂 Requesting airdrop..."
solana airdrop 2

# Check balance
echo "💳 Current balance:"
solana balance

echo ""
echo "✅ Done! You now have test SOL for development."
echo ""
echo "🎯 Next steps:"
echo "1. Make sure Phantom wallet is set to 'Devnet' network"
echo "2. Go to your app: http://localhost:3000/auth"
echo "3. Connect your wallet and authenticate!"
echo ""
echo "💡 Pro tip: You can run this script multiple times to get more SOL"
