<template>
  <v-card class="wallet-auth-card" max-width="500">
    <v-card-title class="text-center">
      <v-icon large color="primary" class="mr-2">mdi-wallet</v-icon>
      Solana Wallet Authentication
    </v-card-title>
    
    <v-alert
      type="info"
      outlined
      dense
      class="ma-4"
    >
      <div class="text-caption">
        <strong>Privacy First:</strong> No wallet information is revealed until you sign a message. 
        Even when connected, your balance and address remain completely hidden until authentication.
      </div>
    </v-alert>

    <v-card-text>
      <!-- RPC Connection Status -->
      <v-alert
        v-if="rpcStatus && !rpcStatus.connected"
        type="warning"
        outlined
        class="mb-4"
      >
        <div class="text-h6 mb-2">Solana Network Connection Issue</div>
        <p>{{ rpcStatus.message }}</p>
        <p class="text-caption mt-2">
          Don't worry! You can still authenticate using a mock balance for development.
        </p>
        <v-btn
          small
          color="primary"
          @click="checkRPCStatus"
          class="mt-2"
        >
          <v-icon left small>mdi-refresh</v-icon>
          Retry Connection
        </v-btn>
      </v-alert>

      <!-- Wallet Not Installed -->
      <v-alert
        v-if="!isPhantomInstalled"
        type="warning"
        outlined
        class="mb-4"
      >
        <div class="text-h6 mb-2">Phantom Wallet Required</div>
        <p>Please install the Phantom wallet extension to continue.</p>
        <v-btn
          color="primary"
          href="https://phantom.app/"
          target="_blank"
          class="mt-2"
        >
          <v-icon left>mdi-download</v-icon>
          Install Phantom Wallet
        </v-btn>
        <div class="mt-3">
          <v-expansion-panels flat>
            <v-expansion-panel>
              <v-expansion-panel-header>
                <span class="text-caption">Need test SOL? Click here for working faucets</span>
              </v-expansion-panel-header>
              <v-expansion-panel-content>
                <div class="text-caption">
                  <p><strong>Working Faucets:</strong></p>
                  <ul>
                    <li><a href="https://solfaucet.com/" target="_blank">SolFaucet (Most reliable)</a></li>
                    <li><a href="https://faucet.quicknode.com/solana/devnet" target="_blank">QuickNode Faucet</a></li>
                    <li><a href="https://solanabeach.io/faucet" target="_blank">Solana Beach</a></li>
                  </ul>
                  <p class="mt-2"><strong>Note:</strong> Make sure to set Phantom to "Devnet" network in settings!</p>
                </div>
              </v-expansion-panel-content>
            </v-expansion-panel>
          </v-expansion-panels>
        </div>
      </v-alert>

      <!-- Not Connected -->
      <div v-else-if="!isConnected" class="text-center">
        <v-icon size="64" color="grey" class="mb-4">mdi-wallet-outline</v-icon>
        <p class="text-h6 mb-4">Connect Your Solana Wallet</p>
        
        <!-- Show message if user was previously authenticated -->
        <v-alert
          v-if="wasPreviouslyAuthenticated"
          type="info"
          outlined
          dense
          class="mb-4"
        >
          <div class="text-caption">
            <strong>Session Expired:</strong> Your wallet was disconnected. Please reconnect to continue.
          </div>
        </v-alert>
        
        <p class="text-body-1 mb-4">
          <span v-if="rpcStatus && rpcStatus.connected">
            Connect your Phantom wallet to begin authentication. Your balance and address will remain hidden until you sign a message.
          </span>
          <span v-else>
            Connect your Phantom wallet or use demo mode to test the authentication flow. No information is revealed until authentication.
          </span>
        </p>
        <v-btn
          color="primary"
          large
          @click="connectWallet"
          :loading="connecting"
          :disabled="connecting"
          class="mb-2"
        >
          <v-icon left>mdi-wallet</v-icon>
          Connect Wallet
        </v-btn>
        <div v-if="!rpcStatus || !rpcStatus.connected" class="mt-2">
          <v-btn
            color="secondary"
            text
            @click="connectWallet"
            :loading="connecting"
          >
            <v-icon left>mdi-play-circle</v-icon>
            Try Demo Mode
          </v-btn>
        </div>
      </div>

      <!-- Connected (but not authenticated) -->
      <div v-else-if="isConnected && walletInfo && !authenticationSuccess" class="text-center">
        <v-icon size="64" color="primary" class="mb-4">mdi-wallet</v-icon>
        <div class="text-h6 mb-2">Wallet Ready for Authentication</div>
        
        <!-- Network Warning -->
        <v-alert
          v-if="!rpcStatus || !rpcStatus.connected"
          type="info"
          outlined
          dense
          class="mb-3"
        >
          <div class="text-caption">
            <strong>Tip:</strong> Make sure your Phantom wallet is set to "Devnet" network in settings for real SOL balance.
          </div>
        </v-alert>
        
        <!-- Hidden Information -->
        <v-card outlined class="mb-4">
          <v-card-text>
            <div class="text-caption text--secondary mb-1">Wallet Information</div>
            <div class="text-body-2 font-mono">
              ••••••••••••••••••••••••••••••••••••••••
            </div>
            <div class="text-caption text--secondary">
              All wallet details are hidden until authentication
            </div>
          </v-card-text>
        </v-card>

        <!-- Actions -->
        <v-btn
          color="success"
          large
          @click="authenticate"
          :loading="authenticating"
          :disabled="authenticating"
          class="mb-2"
        >
          <v-icon left>mdi-signature</v-icon>
          Sign Message & Authenticate
        </v-btn>
        
        <div class="text-caption text--secondary text-center mt-2">
          <v-icon small>mdi-shield-check</v-icon>
          Sign a message to prove wallet ownership and reveal your information
        </div>
        
        <v-btn
          color="grey"
          text
          @click="disconnectWallet"
          :loading="disconnecting"
        >
          <v-icon left>mdi-logout</v-icon>
          Disconnect
        </v-btn>
        
        <v-btn
          color="error"
          text
          small
          @click="forceDisconnect"
          class="mt-2"
        >
          <v-icon left small>mdi-close-circle</v-icon>
          Force Disconnect
        </v-btn>
        
        <div class="text-caption text--secondary mt-2">
          <v-icon small>mdi-information</v-icon>
          If wallet keeps reconnecting, use "Force Disconnect"
        </div>
      </div>

      <!-- Success State (After Authentication) -->
      <div v-if="authenticationSuccess" class="text-center mt-4">
        <v-icon size="64" color="success" class="mb-4">mdi-check-circle</v-icon>
        <div class="text-h5 mb-2 text-success">Authentication Successful!</div>
        
        <!-- Revealed Balance -->
        <v-card outlined class="mb-4">
          <v-card-text>
            <div class="text-caption text--secondary mb-1">Your SOL Balance</div>
            <div class="text-h4 font-weight-bold text-success">
              {{ revealedBalance }}
            </div>
            <div class="text-caption text--secondary">
              Balance revealed after successful authentication
            </div>
          </v-card-text>
        </v-card>
        
        <!-- Revealed Address -->
        <v-card outlined class="mb-4">
          <v-card-text>
            <div class="text-caption text--secondary mb-1">Your Wallet Address</div>
            <div class="text-body-2 font-mono">
              {{ revealedAddress }}
            </div>
          </v-card-text>
        </v-card>
        
        <div class="text-caption text--secondary mb-3">
          <v-icon small>mdi-check-circle</v-icon>
          You are now authenticated and can access all features
        </div>
        
        <v-btn
          color="primary"
          large
          @click="handleRedirectToHome"
          class="mb-2"
        >
          <v-icon left>mdi-home</v-icon>
          Continue to Home
        </v-btn>
        
        <v-btn
          color="grey"
          text
          @click="stayOnAuthPage"
          class="mb-2"
        >
          <v-icon left>mdi-wallet</v-icon>
          Stay Here
        </v-btn>
        
        <v-btn
          color="error"
          text
          small
          @click="disconnectWallet"
        >
          <v-icon left small>mdi-logout</v-icon>
          Disconnect Wallet
        </v-btn>
      </div>

      <!-- Error State -->
      <v-alert
        v-if="error"
        type="error"
        outlined
        class="mt-4"
        dismissible
        @input="error = null"
      >
        {{ error }}
      </v-alert>
    </v-card-text>

    <!-- Loading Overlay for Wallet Connection -->
    <v-overlay
      v-if="connecting"
      :value="connecting"
      absolute
      opacity="0.8"
    >
      <v-card
        class="pa-6 text-center"
        max-width="400"
        outlined
      >
        <v-progress-circular
          indeterminate
          color="primary"
          size="64"
          class="mb-4"
        ></v-progress-circular>
        
        <div class="text-h5 mb-2">Connecting to Wallet...</div>
        
        <div class="text-body-1 mb-4">
          Please check your Phantom wallet extension to approve the connection.
        </div>
        
        <v-alert
          type="info"
          outlined
          dense
        >
          <div class="text-caption">
            <strong>Note:</strong> If the Phantom wallet popup doesn't appear, check your browser's popup blocker settings.
          </div>
        </v-alert>
        
        <v-btn
          color="error"
          text
          @click="cancelConnection"
          class="mt-4"
        >
          <v-icon left>mdi-close</v-icon>
          Cancel
        </v-btn>
      </v-card>
    </v-overlay>

    <!-- Loading Overlay for Authentication -->
    <v-overlay
      v-if="authenticating"
      :value="authenticating"
      absolute
      opacity="0.8"
    >
      <v-card
        class="pa-6 text-center"
        max-width="400"
        outlined
      >
        <v-progress-circular
          indeterminate
          color="primary"
          size="64"
          class="mb-4"
        ></v-progress-circular>
        
        <div class="text-h5 mb-2">Signing Message...</div>
        
        <div class="text-body-1 mb-4">
          Please check your Phantom wallet extension to sign the authentication message.
        </div>
        
        <v-alert
          type="info"
          outlined
          dense
        >
          <div class="text-caption">
            <strong>Note:</strong> If the Phantom wallet popup doesn't appear, check your browser's popup blocker settings.
          </div>
        </v-alert>
        
        <v-btn
          color="error"
          text
          @click="cancelAuthentication"
          class="mt-4"
        >
          <v-icon left>mdi-close</v-icon>
          Cancel
        </v-btn>
      </v-card>
    </v-overlay>
  </v-card>
</template>

<script>
import { solanaService } from '@/services/solana'

export default {
  name: 'WalletAuth',
  data() {
    return {
      isPhantomInstalled: false,
      isConnected: false,
      connecting: false,
      disconnecting: false,
      authenticating: false,
      refreshing: false,
      walletInfo: null,
      error: null,
      rpcStatus: null,
      authenticationSuccess: false,
      revealedBalance: '',
      revealedAddress: ''
    }
  },
  computed: {
    wasPreviouslyAuthenticated() {
      return localStorage.getItem('isAuthenticated') === 'true'
    }
  },
  async mounted() {
    await this.checkRPCStatus()
    await this.checkWalletStatus()
    
    // Listen for wallet disconnect events
    if (window.solana) {
      window.solana.on('disconnect', this.handleWalletDisconnect)
    }
    
    // Listen for page visibility changes
    document.addEventListener('visibilitychange', this.handleVisibilityChange)
    
    // Listen for page unload to clear authentication
    window.addEventListener('beforeunload', this.handlePageUnload)
    
    // Reset success state only if not previously authenticated
    const wasAuthenticated = localStorage.getItem('isAuthenticated') === 'true'
    if (!wasAuthenticated) {
      this.authenticationSuccess = false
      this.revealedBalance = ''
      this.revealedAddress = ''
    }
  },
  
  beforeDestroy() {
    // Remove event listeners
    if (window.solana) {
      window.solana.off('disconnect', this.handleWalletDisconnect)
    }
    document.removeEventListener('visibilitychange', this.handleVisibilityChange)
    window.removeEventListener('beforeunload', this.handlePageUnload)
  },
  methods: {
    async checkRPCStatus() {
      try {
        this.rpcStatus = await solanaService.checkRPCConnection()
      } catch (error) {
        this.rpcStatus = { connected: false, message: 'Failed to check RPC connection' }
      }
    },

    async checkWalletStatus() {
      this.isPhantomInstalled = solanaService.isPhantomInstalled()
      
      // Check if user was previously authenticated
      const wasAuthenticated = localStorage.getItem('isAuthenticated') === 'true'
      
      if (this.isPhantomInstalled && wasAuthenticated) {
        try {
          const status = await solanaService.getConnectionStatus()
          this.isConnected = status.connected
          
          if (this.isConnected) {
            await this.loadWalletInfo(status.publicKey)
            // If previously authenticated, show the success state with revealed values
            if (wasAuthenticated) {
              this.revealedBalance = this.walletInfo.balanceFormatted
              this.revealedAddress = this.walletInfo.publicKey
              this.authenticationSuccess = true
            }
            // If not authenticated, just show connected state (no info revealed)
          } else {
            // Wallet is not connected, clear authentication state
            this.clearAuthenticationState()
          }
        } catch (error) {
          console.error('Error checking wallet status:', error)
          // If there's an error, clear authentication state
          this.clearAuthenticationState()
        }
      } else {
        // If not previously authenticated, don't auto-connect
        this.isConnected = false
        this.walletInfo = null
      }
    },

    async connectWallet() {
      this.connecting = true
      this.error = null
      
      // Set a timeout to cancel connection if it takes too long
      const timeout = setTimeout(() => {
        if (this.connecting) {
          this.connecting = false
          this.error = 'Wallet connection timed out. Please try again.'
        }
      }, 30000) // 30 second timeout
      
      try {
        const result = await solanaService.connectWallet()
        this.isConnected = result.connected
        
        if (this.isConnected) {
          await this.loadWalletInfo(result.publicKey)
          
          // Check if user was previously authenticated
          const wasAuthenticated = localStorage.getItem('isAuthenticated') === 'true'
          if (wasAuthenticated) {
            // If previously authenticated, show the success state
            this.revealedBalance = this.walletInfo.balanceFormatted
            this.revealedAddress = this.walletInfo.publicKey
            this.authenticationSuccess = true
          }
        }
        
        // Clear timeout since connection succeeded
        clearTimeout(timeout)
      } catch (error) {
        clearTimeout(timeout)
        this.error = error.message
      } finally {
        this.connecting = false
      }
    },

    async disconnectWallet() {
      this.disconnecting = true
      this.error = null
      
      try {
        // Force disconnect to clear all connection state
        await solanaService.forceDisconnect()
        this.clearAuthenticationState()
        
        // Emit logout event to parent
        this.$emit('logout')
      } catch (error) {
        this.error = error.message
      } finally {
        this.disconnecting = false
      }
    },

    async forceDisconnect() {
      this.error = null
      
      try {
        // Force disconnect and clear all state
        await solanaService.forceDisconnect()
        this.clearAuthenticationState()
        
        // Emit logout event to parent
        this.$emit('logout')
        
        // Show success message
        this.$emit('show-snackbar', 'Wallet force disconnected successfully', 'info')
      } catch (error) {
        this.error = error.message
      }
    },

    clearAuthenticationState() {
      this.isConnected = false
      this.walletInfo = null
      this.authenticationSuccess = false
      this.revealedBalance = ''
      this.revealedAddress = ''
      localStorage.removeItem('isAuthenticated')
      localStorage.removeItem('solanaUser')
    },

    handleWalletDisconnect() {
      // Handle when user manually disconnects from Phantom wallet
      this.clearAuthenticationState()
      this.$emit('logout')
      this.$emit('show-snackbar', 'Wallet disconnected by user', 'info')
    },

    handleRedirectToHome() {
      this.$emit('redirect-to-home')
    },

    stayOnAuthPage() {
      // User chooses to stay on auth page - no action needed
      // They can see their balance and wallet info here
    },

    cancelAuthentication() {
      // Cancel the authentication process
      this.authenticating = false
      this.error = null
    },

    cancelConnection() {
      // Cancel the wallet connection process
      this.connecting = false
      this.error = null
    },

    handleVisibilityChange() {
      // When page becomes visible again, check if user is still authenticated
      if (!document.hidden) {
        const wasAuthenticated = localStorage.getItem('isAuthenticated') === 'true'
        if (wasAuthenticated && this.isConnected && this.walletInfo) {
          // Restore authenticated state
          this.revealedBalance = this.walletInfo.balanceFormatted
          this.revealedAddress = this.walletInfo.publicKey
          this.authenticationSuccess = true
        }
      }
    },

    handlePageUnload() {
      // When page is being closed, clear authentication state
      // This ensures user has to sign again when they return
      localStorage.removeItem('isAuthenticated')
      localStorage.removeItem('solanaUser')
    },

    async loadWalletInfo(publicKey) {
      try {
        this.walletInfo = await solanaService.getWalletInfo(publicKey)
      } catch (error) {
        this.error = error.message
      }
    },

    async refreshBalance() {
      if (!this.walletInfo) return
      
      this.refreshing = true
      try {
        await this.loadWalletInfo(this.walletInfo.publicKey)
      } catch (error) {
        this.error = error.message
      } finally {
        this.refreshing = false
      }
    },

    async authenticate() {
      this.authenticating = true
      this.error = null
      
      // Set a timeout to cancel authentication if it takes too long
      const timeout = setTimeout(() => {
        if (this.authenticating) {
          this.authenticating = false
          this.error = 'Authentication timed out. Please try again.'
        }
      }, 60000) // 60 second timeout
      
      try {
        // Create authentication message with timestamp
        const timestamp = new Date().toISOString()
        const message = `Authenticate with Sol Backend\nTimestamp: ${timestamp}\nWallet: ${this.walletInfo.publicKey}`
        
        // Sign the message
        const signature = await solanaService.signMessage(message)
        
        if (!signature || !signature.signature) {
          throw new Error('Failed to sign authentication message')
        }
        
        // Clear timeout since authentication succeeded
        clearTimeout(timeout)
        
        // Set revealed values for success state
        this.revealedBalance = this.walletInfo.balanceFormatted
        this.revealedAddress = this.walletInfo.publicKey
        
        // Show success state with revealed balance
        this.authenticationSuccess = true
        
        // Emit authentication event with wallet info
        this.$emit('authenticated', {
          publicKey: this.walletInfo.publicKey,
          balance: this.walletInfo.balance,
          signature: signature.signature,
          message: message,
          timestamp: timestamp
        })
        
      } catch (error) {
        clearTimeout(timeout)
        this.error = error.message
      } finally {
        this.authenticating = false
      }
    },

    formatAddress(address) {
      if (!address) return ''
      return `${address.slice(0, 4)}...${address.slice(-4)}`
    },

    async copyAddress() {
      if (this.walletInfo) {
        try {
          await navigator.clipboard.writeText(this.walletInfo.publicKey)
          this.$emit('show-snackbar', 'Address copied to clipboard!', 'success')
        } catch (error) {
          this.$emit('show-snackbar', 'Failed to copy address', 'error')
        }
      }
    }
  }
}
</script>

<style scoped>
.wallet-auth-card {
  margin: 0 auto;
}

.font-mono {
  font-family: 'Courier New', monospace;
}
</style>
