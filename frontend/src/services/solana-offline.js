// Offline Solana service for development when RPC is not available
export const solanaOfflineService = {
  // Mock wallet connection
  async connectWallet() {
    // Simulate wallet connection delay
    await new Promise(resolve => setTimeout(resolve, 1000))
    
    return {
      publicKey: 'DemoWallet123456789012345678901234567890',
      connected: true
    }
  },

  // Mock balance
  async getBalance() {
    // Return random balance between 0.1 and 5 SOL
    return Math.random() * 4.9 + 0.1
  },

  // Mock wallet info
  async getWalletInfo(publicKey) {
    const balance = await this.getBalance()
    return {
      publicKey: publicKey,
      balance: balance,
      balanceFormatted: `${balance.toFixed(4)} SOL (Demo)`
    }
  },

  // Mock message signing
  async signMessage(message) {
    await new Promise(resolve => setTimeout(resolve, 500))
    return {
      signature: Array.from({ length: 64 }, () => Math.floor(Math.random() * 256)),
      publicKey: 'DemoWallet123456789012345678901234567890'
    }
  },

  // Mock connection status
  async getConnectionStatus() {
    return {
      connected: false,
      publicKey: null
    }
  },

  // Mock disconnect
  async disconnectWallet() {
    return { connected: false }
  },

  // Check if "wallet" is installed (always true for demo)
  isPhantomInstalled() {
    return true
  }
}

export default solanaOfflineService
