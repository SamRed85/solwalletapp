# Solana Blockchain Authentication

This application now supports authentication using Solana blockchain wallets! Users can connect their Phantom wallet and authenticate by checking their SOL balance.

## 🚀 Features

### Frontend (Vue.js)
- **Phantom Wallet Integration**: Connect to Phantom wallet extension
- **Real-time Balance Checking**: Display current SOL balance
- **Wallet Authentication**: Sign messages for secure authentication
- **Persistent Sessions**: Store authentication state in localStorage
- **Beautiful UI**: Material Design interface with Vuetify

### Backend (Go + MongoDB)
- **Solana Authentication Endpoint**: `/api/auth/solana`
- **Wallet Address Storage**: Store user wallet addresses in MongoDB
- **Balance Tracking**: Track and update SOL balances
- **User Management**: Create/update users based on wallet addresses

## 🔧 Setup

### 1. Install Phantom Wallet
Users need to install the Phantom wallet browser extension:
- **Chrome**: https://chrome.google.com/webstore/detail/phantom/bfnaelmomeimhlpmgjnjophhpkkoljpa
- **Firefox**: https://addons.mozilla.org/en-US/firefox/addon/phantom-app/
- **Website**: https://phantom.app/

### 2. Get Test SOL (Devnet)
For testing, users can get free SOL on Solana devnet using these working faucets:

**Primary Options:**
- **SolFaucet**: https://solfaucet.com/ (Most reliable)
- **Solana CLI**: `solana airdrop 2 <wallet-address> --url devnet`
- **QuickNode Faucet**: https://faucet.quicknode.com/solana/devnet

**Alternative Options:**
- **Solana Cookbook Faucet**: https://cookbook.solana.com/getting-started/installation#airdrop
- **Solana Beach Faucet**: https://solanabeach.io/faucet
- **Solana Labs Faucet**: https://faucet.solana.com/ (Often unreliable)

**Using Solana CLI (Recommended):**
```bash
# Install Solana CLI first
sh -c "$(curl -sSfL https://release.solana.com/v1.17.0/install)"

# Add to PATH (restart terminal)
export PATH="$HOME/.local/share/solana/install/active_release/bin:$PATH"

# Airdrop SOL to your wallet
solana airdrop 2 <your-wallet-address> --url devnet
```

### 3. Run the Application

```bash
# Start MongoDB
docker-compose up -d

# Start Backend
cd backend
go run main.go

# Start Frontend
cd frontend
npm install
npm run serve
```

## 🎯 How It Works

### Authentication Flow
1. **User visits `/auth` page**
2. **Clicks "Connect Wallet"** - Connects to Phantom wallet
3. **No information revealed** - Even when connected, balance and address remain completely hidden
4. **Clicks "Sign Message & Authenticate"** - Creates unique message with timestamp
5. **User signs message** - Proves wallet ownership with cryptographic signature
6. **Backend validates signature** - Verifies message signature and timestamp (5 min expiry)
7. **Backend creates/updates user** - Stores wallet address and balance
8. **Information revealed** - Only after successful authentication are balance and address shown
9. **User is authenticated** - Can access protected features with revealed information
10. **Persistent authentication** - User stays authenticated until page is closed or wallet is disconnected

### Technical Details
- **Network**: Solana Devnet (for development)
- **RPC Endpoint**: `https://api.devnet.solana.com`
- **Wallet**: Phantom wallet extension
- **Authentication**: Message signing with wallet (required once per session)
- **Security**: Timestamp validation (5-minute expiry) and signature verification
- **Privacy First**: No wallet information revealed until message signing
- **Zero Information Leakage**: Even when connected, balance and address remain hidden
- **Session Persistence**: Authentication persists until page is closed or wallet is disconnected
- **Storage**: MongoDB with wallet address as unique identifier

## 📊 API Endpoints

### Solana Authentication
```http
POST /api/auth/solana
Content-Type: application/json

{
  "publicKey": "wallet_address_here",
  "balance": 1.5,
  "signature": [1, 2, 3, ...],
  "message": "authentication_message"
}
```

**Response:**
```json
{
  "success": true,
  "user": {
    "id": "user_id",
    "name": "Solana User",
    "email": "wallet@solana.wallet",
    "walletAddress": "wallet_address",
    "solBalance": 1.5,
    "lastLogin": "2025-01-17T10:30:00Z"
  },
  "message": "User authenticated successfully",
  "authenticatedAt": "2025-01-17T10:30:00Z"
}
```

## 🗄️ Database Schema

### Users Collection
```javascript
{
  _id: ObjectId,
  name: String,
  email: String,
  walletAddress: String, // Unique Solana wallet address
  solBalance: Number,    // Current SOL balance
  lastLogin: Date       // Last authentication time
}
```

### Indexes
- `email`: Unique index
- `walletAddress`: Unique index

## 🎨 User Interface

### Authentication Page (`/auth`)
- **Wallet Installation Guide**: Instructions for Phantom wallet
- **Connection Status**: Shows if wallet is connected
- **Balance Display**: Real-time SOL balance
- **Authentication Button**: Signs message and authenticates

### App Bar
- **Balance Chip**: Shows current SOL balance when authenticated
- **Connect Button**: Quick access to authentication when not logged in

### Navigation
- **Wallet Auth Menu Item**: Direct access to authentication page

## 🔒 Security Features

- **Message Signing**: Users sign authentication messages with their private key
- **Unique Wallet Addresses**: Each wallet address can only have one account
- **Balance Verification**: Real-time balance checking from Solana network
- **Session Management**: Secure localStorage for authentication state

## 🚀 Production Considerations

### For Production Deployment:
1. **Switch to Mainnet**: Change RPC endpoint to mainnet
2. **Add Signature Verification**: Verify message signatures on backend
3. **Rate Limiting**: Add rate limiting to authentication endpoint
4. **HTTPS**: Ensure all connections use HTTPS
5. **Environment Variables**: Use environment variables for RPC endpoints

### Environment Variables:
```bash
# Backend
MONGODB_URI=mongodb://localhost:27017
SOLANA_RPC_URL=https://api.mainnet-beta.solana.com

# Frontend
VUE_APP_SOLANA_RPC_URL=https://api.mainnet-beta.solana.com
```

## 🎉 Benefits

- **No Passwords**: Users don't need to remember passwords
- **Decentralized**: Authentication through blockchain
- **Real-time Balance**: Always up-to-date SOL balance
- **Secure**: Private keys never leave the user's wallet
- **User-friendly**: Simple wallet connection flow

## 🔧 Troubleshooting

### Common Issues:
1. **Phantom Wallet Not Installed**: Show installation instructions
2. **Wallet Not Connected**: Guide user to connect wallet
3. **Insufficient Balance**: Show balance and suggest getting test SOL
4. **Network Issues**: Check Solana RPC endpoint connectivity

### Development Tips:
- Use Solana devnet for testing
- Get free SOL from faucet for testing
- Check browser console for wallet connection errors
- Verify MongoDB connection for user storage

This Solana authentication system provides a modern, secure, and user-friendly way to authenticate users using their blockchain wallets! 🎨✨
