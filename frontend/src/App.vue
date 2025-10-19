<template>
  <v-app>
    <v-app-bar
      app
      color="primary"
      :dark="darkTheme"
    >
      <v-app-bar-nav-icon @click="drawer = !drawer"></v-app-bar-nav-icon>
      
      <v-toolbar-title>Sol Frontend</v-toolbar-title>

          <v-spacer></v-spacer>

          <!-- Wallet Balance (Top Right) -->
          <v-chip
            v-if="isAuthenticated"
            color="accent"
            text-color="white"
            class="mr-2"
            small
          >
            <v-icon left>mdi-wallet</v-icon>
            {{ userBalance }} SOL
          </v-chip>

          <!-- Username Display -->
          <v-chip
            v-if="isAuthenticated"
            :color="isAdmin ? 'warning' : 'success'"
            text-color="white"
            class="mr-2"
            small
          >
            <v-icon left small>{{ isAdmin ? 'mdi-shield-crown' : 'mdi-account' }}</v-icon>
            {{ username }}
            <v-icon v-if="isAdmin" right small>mdi-star</v-icon>
          </v-chip>

          <!-- Admin Dropdown Menu -->
          <v-menu
            v-if="isAuthenticated && isAdmin"
            bottom
            left
            offset-y
          >
            <template v-slot:activator="{ on, attrs }">
              <v-btn
                color="warning"
                text
                v-bind="attrs"
                v-on="on"
                class="mr-2"
              >
                <v-icon left>mdi-cog</v-icon>
                Admin
                <v-icon right>mdi-chevron-down</v-icon>
              </v-btn>
            </template>
            
            <v-list>
              <v-list-item @click="$router.push('/admin')">
                <v-list-item-icon>
                  <v-icon>mdi-shield-crown</v-icon>
                </v-list-item-icon>
                <v-list-item-content>
                  <v-list-item-title>Admin Dashboard</v-list-item-title>
                </v-list-item-content>
              </v-list-item>
              
              <v-list-item @click="$router.push('/users')">
                <v-list-item-icon>
                  <v-icon>mdi-account-group</v-icon>
                </v-list-item-icon>
                <v-list-item-content>
                  <v-list-item-title>Manage Users</v-list-item-title>
                </v-list-item-content>
              </v-list-item>
              
              <v-divider></v-divider>
              
              <v-list-item disabled>
                <v-list-item-icon>
                  <v-icon>mdi-database</v-icon>
                </v-list-item-icon>
                <v-list-item-content>
                  <v-list-item-title>Database Stats</v-list-item-title>
                  <v-list-item-subtitle>Coming Soon</v-list-item-subtitle>
                </v-list-item-content>
              </v-list-item>
              
              <v-list-item disabled>
                <v-list-item-icon>
                  <v-icon>mdi-cog</v-icon>
                </v-list-item-icon>
                <v-list-item-content>
                  <v-list-item-title>System Settings</v-list-item-title>
                  <v-list-item-subtitle>Coming Soon</v-list-item-subtitle>
                </v-list-item-content>
              </v-list-item>
              
              <v-list-item disabled>
                <v-list-item-icon>
                  <v-icon>mdi-security</v-icon>
                </v-list-item-icon>
                <v-list-item-content>
                  <v-list-item-title>Security Logs</v-list-item-title>
                  <v-list-item-subtitle>Coming Soon</v-list-item-subtitle>
                </v-list-item-content>
              </v-list-item>
            </v-list>
          </v-menu>

          <!-- Logout Button (Top Right) -->
          <v-btn
            v-if="isAuthenticated"
            color="error"
            text
            @click="logout"
            class="mr-2"
          >
            <v-icon left>mdi-logout</v-icon>
            Logout
          </v-btn>

          <!-- Connect Wallet Button (Top Right) -->
          <v-btn
            v-if="!isAuthenticated"
            color="success"
            outlined
            @click="connectWallet"
            :loading="connecting"
            :disabled="connecting"
            class="mr-2"
            large
          >
            <v-icon left>mdi-wallet</v-icon>
            Connect
          </v-btn>

          <!-- Wallet Status for Non-Authenticated -->
          <v-chip
            v-if="!isAuthenticated"
            color="grey"
            text-color="white"
            class="mr-2"
            small
          >
            <v-icon left small>mdi-wallet-outline</v-icon>
            Not Connected
          </v-chip>

    </v-app-bar>

    <v-navigation-drawer
      v-model="drawer"
      app
      :dark="darkTheme"
    >
      <v-list>
        <v-list-item
          v-for="item in filteredMenuItems"
          :key="item.title"
          :to="item.to"
          link
        >
          <v-list-item-icon>
            <v-icon>{{ item.icon }}</v-icon>
          </v-list-item-icon>

          <v-list-item-content>
            <v-list-item-title>{{ item.title }}</v-list-item-title>
          </v-list-item-content>
        </v-list-item>
      </v-list>
    </v-navigation-drawer>

    <v-main>
      <router-view />
    </v-main>

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

    <v-footer
      color="primary"
      :dark="darkTheme"
      padless
    >
      <v-row
        justify="center"
        no-gutters
      >
        <v-btn
          v-for="link in footerLinks"
          :key="link"
          color="white"
          text
          rounded
          class="my-2"
        >
          {{ link }}
        </v-btn>
        <v-col
          class="text-center mt-4"
          cols="12"
        >
          {{ new Date().getFullYear() }} — <strong>Sol Frontend</strong>
        </v-col>
      </v-row>
    </v-footer>

    <!-- Bottom Bar with Backend Status and Theme Toggle -->
    <v-bottom-navigation
      app
      :color="darkTheme ? 'grey darken-4' : 'grey lighten-2'"
      :dark="darkTheme"
      height="48"
    >
      <v-btn
        :color="healthStatus && healthStatus.status === 'healthy' ? 'success' : 'error'"
        text
        disabled
      >
        <v-icon left>{{ healthStatus && healthStatus.status === 'healthy' ? 'mdi-check-circle' : 'mdi-alert-circle' }}</v-icon>
        <span class="text-caption">
          Backend: {{ healthStatus ? healthStatus.status : 'checking...' }}
        </span>
      </v-btn>
      
      <v-spacer></v-spacer>
      
      <!-- Theme Toggle Switch -->
      <v-btn
        :color="darkTheme ? 'primary' : 'grey'"
        text
        @click="toggleTheme"
        class="mr-2"
      >
        <v-icon left>{{ themeIcon }}</v-icon>
        <span class="text-caption">{{ themeLabel }}</span>
      </v-btn>
      
      <v-btn
        color="grey"
        text
        disabled
      >
        <v-icon left>mdi-server</v-icon>
        <span class="text-caption">Sol API</span>
      </v-btn>
    </v-bottom-navigation>

    <!-- Snackbar for notifications -->
    <v-snackbar
      v-model="snackbar.show"
      :color="snackbar.color"
      :timeout="snackbar.timeout"
      top
    >
      {{ snackbar.text }}
      <template v-slot:action="{ attrs }">
        <v-btn
          color="white"
          text
          v-bind="attrs"
          @click="snackbar.show = false"
        >
          Close
        </v-btn>
      </template>
    </v-snackbar>
  </v-app>
</template>

<script>
import { solanaService } from '@/services/solana'
import { authService } from '@/services/api'

export default {
  name: 'App',
  data: () => ({
    drawer: false,
    isAuthenticated: false,
    userBalance: '0.0000',
    connecting: false,
    authenticating: false,
    isAdmin: false,
    healthStatus: null,
    healthCheckInterval: null,
    darkTheme: false,
    snackbar: {
      show: false,
      text: '',
      color: 'success',
      timeout: 5000
    },
        menuItems: [
          { title: 'Home', icon: 'mdi-home', to: '/' },
          { title: 'Predictions', icon: 'mdi-target', to: '/predictions' },
          { title: 'My Coupons', icon: 'mdi-ticket', to: '/my-coupons' },
          { title: 'Users', icon: 'mdi-account-group', to: '/users', requiresAdmin: true },
          { title: 'Admin', icon: 'mdi-shield-crown', to: '/admin', requiresAdmin: true }
        ],
    footerLinks: [
      'Home',
      'About Us',
      'Contact'
    ]
  }),
  computed: {
    username() {
      if (this.isAuthenticated) {
        const userData = localStorage.getItem('solanaUser')
        if (userData) {
          try {
            const user = JSON.parse(userData)
            // Use wallet address as username, truncated for display
            return user.walletAddress ? `${user.walletAddress.slice(0, 4)}...${user.walletAddress.slice(-4)}` : 'User'
          } catch (error) {
            return 'User'
          }
        }
      }
      return null
    },
    themeIcon() {
      return this.darkTheme ? 'mdi-weather-night' : 'mdi-weather-sunny'
    },
    themeLabel() {
      return this.darkTheme ? 'Dark' : 'Light'
    },
    
    filteredMenuItems() {
      return this.menuItems.filter(item => {
        if (item.requiresAdmin) {
          // Only show admin menu to authenticated admin users
          return this.isAuthenticated && this.isAdmin
        }
        return true
      })
    }
  },
  mounted() {
    this.checkAuthenticationStatus()
    this.checkBackendHealth()
    
    // Initialize theme from localStorage
    const savedTheme = localStorage.getItem('darkTheme')
    if (savedTheme !== null) {
      this.darkTheme = savedTheme === 'true'
      this.$vuetify.theme.dark = this.darkTheme
    }
    
    // Check backend health every 30 seconds
    this.healthCheckInterval = setInterval(() => {
      this.checkBackendHealth()
    }, 30000)
    
    // Listen for storage changes (when user logs out in another tab)
    window.addEventListener('storage', this.handleStorageChange)
  },
  
  beforeDestroy() {
    window.removeEventListener('storage', this.handleStorageChange)
    if (this.healthCheckInterval) {
      clearInterval(this.healthCheckInterval)
    }
  },
  methods: {
    async checkAuthenticationStatus() {
      const isAuth = localStorage.getItem('isAuthenticated') === 'true'
      const userData = localStorage.getItem('solanaUser')
      
      this.isAuthenticated = isAuth
      
      if (isAuth && userData) {
        try {
          const user = JSON.parse(userData)
          this.userBalance = user.solBalance ? user.solBalance.toFixed(4) : '0.0000'
          
          // Check admin status
          await this.checkAdminStatus()
        } catch (error) {
          console.error('Error parsing user data:', error)
          this.logout()
        }
      } else {
        this.isAdmin = false
      }
    },
    
    async checkAdminStatus() {
      try {
        const { adminService } = await import('@/services/api')
        const adminStatus = await adminService.checkAdminStatus()
        this.isAdmin = adminStatus.isAdmin
      } catch (error) {
        console.error('Admin check failed:', error)
        this.isAdmin = false
      }
    },

    async checkBackendHealth() {
      try {
        const { healthService } = await import('@/services/api')
        const response = await healthService.checkHealth()
        console.log('Health check response:', response)
        this.healthStatus = response
      } catch (error) {
        console.error('Health check error:', error)
        this.healthStatus = {
          status: 'unhealthy',
          message: 'Backend is not responding'
        }
      }
    },

    toggleTheme() {
      this.darkTheme = !this.darkTheme
      this.$vuetify.theme.dark = this.darkTheme
      localStorage.setItem('darkTheme', this.darkTheme.toString())
    },

    // Safe navigation helper to avoid redundant navigation errors
    safeNavigate(path) {
      if (this.$route.path !== path) {
        this.$router.push(path).catch(err => {
          // Ignore navigation duplicated errors
          if (err.name !== 'NavigationDuplicated') {
            console.error('Navigation error:', err)
          }
        })
      }
    },

    logout() {
      localStorage.removeItem('isAuthenticated')
      localStorage.removeItem('solanaUser')
      this.isAuthenticated = false
      this.userBalance = '0.0000'
      this.isAdmin = false
      // Disconnect wallet
      solanaService.forceDisconnect()
      
      // Refresh page to clear all cached data
      window.location.reload()
    },

    async connectWallet() {
      this.connecting = true
      
      // Set a timeout to cancel connection if it takes too long
      const timeout = setTimeout(() => {
        if (this.connecting) {
          this.connecting = false
          this.showSnackbar('Wallet connection timed out. Please try again.', 'error')
        }
      }, 30000) // 30 second timeout
      
      try {
        // Connect to wallet
        const result = await solanaService.connectWallet()
        
        if (result.connected) {
          // Get wallet info
          const walletInfo = await solanaService.getWalletInfo(result.publicKey)
          
          // Clear connection timeout
          clearTimeout(timeout)
          this.connecting = false
          
          // Start authentication process
          await this.authenticate(walletInfo)
        } else {
          clearTimeout(timeout)
          this.connecting = false
          this.showSnackbar('Failed to connect to wallet', 'error')
        }
      } catch (error) {
        clearTimeout(timeout)
        this.connecting = false
        this.showSnackbar(`Connection error: ${error.message}`, 'error')
      }
    },

    async authenticate(walletInfo) {
      this.authenticating = true
      
      // Set a timeout to cancel authentication if it takes too long
      const timeout = setTimeout(() => {
        if (this.authenticating) {
          this.authenticating = false
          this.showSnackbar('Authentication timed out. Please try again.', 'error')
        }
      }, 60000) // 60 second timeout
      
      try {
        // Create authentication message with timestamp
        const timestamp = new Date().toISOString()
        const message = `Authenticate with Sol Backend\nTimestamp: ${timestamp}\nWallet: ${walletInfo.publicKey}`
        
        // Sign the message
        const signature = await solanaService.signMessage(message)
        
        if (!signature || !signature.signature) {
          throw new Error('Failed to sign authentication message')
        }
        
        // Clear authentication timeout
        clearTimeout(timeout)
        
        // Send authentication to backend
        const authData = {
          publicKey: walletInfo.publicKey,
          balance: walletInfo.balance,
          signature: signature.signature,
          message: message,
          timestamp: timestamp
        }
        
        const response = await authService.authenticateWithSolana(authData)
        
        if (response.success) {
          // Store user data in localStorage
          localStorage.setItem('solanaUser', JSON.stringify(response.user))
          localStorage.setItem('isAuthenticated', 'true')
          
          // Update UI
          this.isAuthenticated = true
          this.userBalance = response.user.solBalance ? response.user.solBalance.toFixed(4) : '0.0000'
          
          // Check admin status after authentication
          await this.checkAdminStatus()
          
          this.showSnackbar('Authentication successful! Welcome to Sol!', 'success')
          
          // Refresh page to clear all cached data and ensure clean state
          setTimeout(() => {
            window.location.reload()
          }, 1000) // Small delay to show success message
        } else {
          this.showSnackbar('Authentication failed. Please try again.', 'error')
        }
      } catch (error) {
        clearTimeout(timeout)
        this.showSnackbar(`Authentication error: ${error.message}`, 'error')
      } finally {
        this.authenticating = false
      }
    },

    cancelConnection() {
      this.connecting = false
    },

    cancelAuthentication() {
      this.authenticating = false
    },

    showSnackbar(text, color = 'success') {
      this.snackbar.text = text
      this.snackbar.color = color
      this.snackbar.show = true
    },

    handleStorageChange(event) {
      // Handle storage changes from other tabs
      if (event.key === 'isAuthenticated' || event.key === 'solanaUser') {
        this.checkAuthenticationStatus()
      }
    }
  }
}
</script>
