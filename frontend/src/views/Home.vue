<template>
  <v-container>
    <v-row justify="center" align="center" class="fill-height">
      <v-col cols="12" md="8" lg="6">
        <div class="text-center">
          <!-- Welcome Message -->
          <v-icon size="80" color="primary" class="mb-6">mdi-home</v-icon>
          
          <h1 class="text-h3 font-weight-light mb-4">
            Welcome to Sol
          </h1>
          
          <p class="text-h6 text--secondary mb-8">
            Your gateway to Solana blockchain authentication
          </p>
          
          <!-- User-specific greeting -->
          <div v-if="isAuthenticated" class="mb-6">
            <v-card outlined class="pa-6">
              <v-icon size="48" color="success" class="mb-4">mdi-check-circle</v-icon>
              <h2 class="text-h5 mb-2">Hello, {{ username }}!</h2>
              <p class="text-body-1 text--secondary">
                You're successfully authenticated and ready to explore.
              </p>
            </v-card>
          </div>
          
          <!-- Non-authenticated greeting -->
          <div v-else class="mb-6">
            <v-card outlined class="pa-6">
              <v-icon size="48" color="primary" class="mb-4">mdi-wallet</v-icon>
              <h2 class="text-h5 mb-2">Ready to get started?</h2>
              <p class="text-body-1 text--secondary">
                Connect your Solana wallet to begin your journey.
              </p>
            </v-card>
          </div>
          
          <!-- Quick actions for authenticated users -->
          <div v-if="isAuthenticated" class="mt-8">
            <v-row justify="center">
              <v-col v-if="isAdmin" cols="12" sm="6">
                <v-btn
                  color="primary"
                  large
                  block
                  @click="$router.push('/admin')"
                  class="mb-3"
                >
                  <v-icon left>mdi-shield-crown</v-icon>
                  Admin Dashboard
                </v-btn>
              </v-col>
              
              <v-col v-if="isAdmin" cols="12" sm="6">
                <v-btn
                  color="secondary"
                  large
                  block
                  @click="$router.push('/users')"
                  class="mb-3"
                >
                  <v-icon left>mdi-account-group</v-icon>
                  Manage Users
                </v-btn>
              </v-col>
            </v-row>
          </div>
        </div>
      </v-col>
    </v-row>
  </v-container>
</template>

<script>
export default {
  name: 'Home',
  data() {
    return {
      isAdmin: false
    }
  },
  computed: {
    isAuthenticated() {
      return localStorage.getItem('isAuthenticated') === 'true'
    },
    username() {
      const userData = localStorage.getItem('solanaUser')
      if (userData) {
        try {
          const user = JSON.parse(userData)
          return user.walletAddress ? `${user.walletAddress.slice(0, 4)}...${user.walletAddress.slice(-4)}` : 'User'
        } catch (error) {
          return 'User'
        }
      }
      return 'User'
    }
  },
  async mounted() {
    await this.checkAdminStatus()
  },
  methods: {
    async checkAdminStatus() {
      try {
        const isAuthenticated = localStorage.getItem('isAuthenticated') === 'true'
        if (isAuthenticated) {
          const { adminService } = await import('@/services/api')
          const adminStatus = await adminService.checkAdminStatus()
          this.isAdmin = adminStatus.isAdmin
        }
      } catch (error) {
        console.error('Admin check failed:', error)
        this.isAdmin = false
      }
    }
  }
}
</script>
