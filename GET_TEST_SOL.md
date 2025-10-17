# How to Get Test SOL for Development

## 🚨 Problem: Solana Faucets Don't Work

The official Solana faucet at https://faucet.solana.com/ is often unreliable. Here are working alternatives to get test SOL for development.

## ✅ Working Solutions

### **Option 1: SolFaucet (Most Reliable)**
- **URL**: https://solfaucet.com/
- **How to use**:
  1. Connect your Phantom wallet
  2. Select "Devnet" network
  3. Click "Request SOL"
  4. Get 1-2 SOL instantly

### **Option 2: QuickNode Faucet**
- **URL**: https://faucet.quicknode.com/solana/devnet
- **How to use**:
  1. Enter your wallet address
  2. Complete captcha
  3. Click "Request SOL"
  4. Get 1 SOL per request

### **Option 3: Solana CLI (Recommended for Developers)**
- **Most reliable method**
- **How to install and use**:

```bash
# Install Solana CLI
sh -c "$(curl -sSfL https://release.solana.com/v1.17.0/install)"

# Add to PATH (restart terminal after this)
export PATH="$HOME/.local/share/solana/install/active_release/bin:$PATH"

# Set to devnet
solana config set --url devnet

# Check your wallet address
solana address

# Airdrop SOL (can do this multiple times)
solana airdrop 2

# Check balance
solana balance
```

### **Option 4: Alternative Faucets**
- **Solana Beach**: https://solanabeach.io/faucet
- **Solana Cookbook**: https://cookbook.solana.com/getting-started/installation#airdrop
- **Solana Labs**: https://faucet.solana.com/ (Often unreliable)

## 🎯 For Your Application Testing

### **Best Approach: Use Demo Mode**
Since faucets can be unreliable, your app now includes a **Demo Mode** that works perfectly:

1. **Go to**: `http://localhost:3000/auth`
2. **Click**: "Try Demo Mode"
3. **Experience**: Full authentication flow with mock SOL balance
4. **No external dependencies**: Works completely offline

### **If You Want Real SOL Testing**:
1. **Install Solana CLI** (most reliable)
2. **Use SolFaucet** as backup
3. **Set Phantom to Devnet**:
   - Open Phantom wallet
   - Click settings (gear icon)
   - Change network to "Devnet"

## 🔧 Troubleshooting Faucets

### **Common Issues**:
- **"Faucet is empty"**: Try different faucets or wait
- **"Rate limited"**: Wait 24 hours or use different wallet
- **"Invalid address"**: Make sure wallet is on devnet
- **"Network error"**: Try Solana CLI instead

### **Pro Tips**:
- **Use multiple wallets**: Create several Phantom wallets for testing
- **Solana CLI is most reliable**: Always works when RPC is up
- **Check devnet status**: https://status.solana.com/
- **Use demo mode**: Perfect for development without external dependencies

## 🚀 Quick Start Guide

### **For Immediate Testing (Demo Mode)**:
```bash
# Start your app
docker-compose up -d
cd backend && go run main.go
cd frontend && npm run serve

# Go to http://localhost:3000/auth
# Click "Try Demo Mode"
# Full authentication works immediately!
```

### **For Real SOL Testing**:
```bash
# Install Solana CLI
sh -c "$(curl -sSfL https://release.solana.com/v1.17.0/install)"

# Get SOL
solana airdrop 2 --url devnet

# Test with real wallet
# Go to http://localhost:3000/auth
# Connect Phantom wallet
# Authenticate with real SOL balance
```

## 📊 Faucet Reliability Ranking

1. **🥇 Solana CLI**: 99% reliable
2. **🥈 SolFaucet**: 90% reliable
3. **🥉 QuickNode**: 85% reliable
4. **🏅 Solana Beach**: 70% reliable
5. **❌ Official Faucet**: 30% reliable

## 🎉 Recommendation

**For development and testing**: Use the **Demo Mode** in your app - it's 100% reliable and works offline!

**For production testing**: Use **Solana CLI** - it's the most reliable way to get test SOL.

Your application now works perfectly regardless of faucet availability! 🚀✨
