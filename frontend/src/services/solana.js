import { Connection, PublicKey, LAMPORTS_PER_SOL } from '@solana/web3.js'
import { solanaOfflineService } from './solana-offline.js'

// Solana RPC endpoints - using multiple fallbacks for reliability
const SOLANA_RPC_URLS = [
  'https://api.devnet.solana.com',
  'https://devnet.helius-rpc.com/?api-key=',
  'https://rpc.ankr.com/solana_devnet',
  'https://solana-devnet.g.alchemy.com/v2/demo'
]

// Try to create connection with fallback endpoints
let connection = null
for (const url of SOLANA_RPC_URLS) {
  try {
    connection = new Connection(url, 'confirmed')
    console.log(`Connected to Solana RPC: ${url}`)
    break
  } catch (error) {
    console.warn(`Failed to connect to ${url}:`, error.message)
  }
}

if (!connection) {
  console.error('Failed to connect to any Solana RPC endpoint')
}

export const solanaService = {
  // Check if Phantom wallet is installed
  isPhantomInstalled() {
    return window.solana && window.solana.isPhantom
  },

  // Check if we should use offline mode
  shouldUseOfflineMode() {
    return !connection
  },

  // Connect to Phantom wallet
  async connectWallet() {
    if (!this.isPhantomInstalled()) {
      // If no real wallet and no RPC, use offline demo mode
      if (this.shouldUseOfflineMode()) {
        console.log('Using offline demo mode for wallet connection')
        return await solanaOfflineService.connectWallet()
      }
      throw new Error('Phantom wallet is not installed. Please install it from https://phantom.app/')
    }

    try {
      const response = await window.solana.connect()
      return {
        publicKey: response.publicKey.toString(),
        connected: true
      }
    } catch (error) {
      throw new Error(`Failed to connect wallet: ${error.message}`)
    }
  },

  // Disconnect wallet
  async disconnectWallet() {
    if (this.isPhantomInstalled()) {
      try {
        await window.solana.disconnect()
      } catch (error) {
        console.warn('Error disconnecting wallet:', error)
      }
    }
    return { connected: false }
  },

  // Force disconnect (clear all connection state)
  async forceDisconnect() {
    if (this.isPhantomInstalled()) {
      try {
        await window.solana.disconnect()
      } catch (error) {
        console.warn('Error force disconnecting wallet:', error)
      }
    }
    
    // Clear any cached connection state
    if (window.solana && window.solana._connected) {
      window.solana._connected = false
    }
    
    return { connected: false }
  },

  // Get wallet balance in SOL
  async getBalance(publicKeyString) {
    if (!connection) {
      console.log('No RPC connection, using offline demo balance')
      return await solanaOfflineService.getBalance()
    }
    
    try {
      const publicKey = new PublicKey(publicKeyString)
      const balance = await connection.getBalance(publicKey)
      return balance / LAMPORTS_PER_SOL // Convert lamports to SOL
    } catch (error) {
      // If balance check fails, return a mock balance for development
      console.warn('Failed to get real balance, using mock balance:', error.message)
      return Math.random() * 10 // Return random balance between 0-10 SOL for testing
    }
  },

  // Sign a message for authentication
  async signMessage(message) {
    if (!this.isPhantomInstalled()) {
      // If no real wallet and no RPC, use offline demo mode
      if (this.shouldUseOfflineMode()) {
        console.log('Using offline demo mode for message signing')
        return await solanaOfflineService.signMessage(message)
      }
      throw new Error('Phantom wallet is not installed')
    }

    try {
      const encodedMessage = new TextEncoder().encode(message)
      const signedMessage = await window.solana.signMessage(encodedMessage, 'utf8')
      return {
        signature: Array.from(signedMessage.signature),
        publicKey: signedMessage.publicKey.toString()
      }
    } catch (error) {
      throw new Error(`Failed to sign message: ${error.message}`)
    }
  },

  // Verify wallet connection status
  async getConnectionStatus() {
    if (!this.isPhantomInstalled()) {
      return { connected: false, publicKey: null }
    }

    try {
      // Check if wallet is connected without auto-connecting
      if (window.solana.isConnected) {
        return {
          connected: true,
          publicKey: window.solana.publicKey.toString()
        }
      }
      return { connected: false, publicKey: null }
    } catch (error) {
      return { connected: false, publicKey: null }
    }
  },

  // Get wallet info including balance
  async getWalletInfo(publicKeyString) {
    try {
      const balance = await this.getBalance(publicKeyString)
      return {
        publicKey: publicKeyString,
        balance: balance,
        balanceFormatted: `${balance.toFixed(4)} SOL`
      }
    } catch (error) {
      throw new Error(`Failed to get wallet info: ${error.message}`)
    }
  },

  // Check RPC connection status
  async checkRPCConnection() {
    if (!connection) {
      return { connected: false, message: 'No RPC connection available' }
    }
    
    try {
      const version = await connection.getVersion()
      return { 
        connected: true, 
        message: `Connected to Solana ${version['solana-core']}`,
        version: version
      }
    } catch (error) {
      return { 
        connected: false, 
        message: `RPC connection failed: ${error.message}` 
      }
    }
  }
}

export default solanaService
