<template>
  <v-container>
    <v-row>
      <v-col cols="12">
        <!-- Not Authenticated -->
        <v-card v-if="!isAuthenticated">
          <v-card-title>
            <v-icon class="mr-2">mdi-account-group</v-icon>
            User Management
          </v-card-title>
          <v-card-text class="text-center pa-8">
            <v-icon size="64" color="grey" class="mb-4">mdi-lock</v-icon>
            <h3 class="mb-4">Authentication Required</h3>
            <p class="text-body-1 mb-4">
              You need to authenticate with your Solana wallet to view users.
            </p>
            <v-btn
              color="primary"
              large
              @click="$router.push('/')"
            >
              <v-icon left>mdi-wallet</v-icon>
              Connect Wallet
            </v-btn>
          </v-card-text>
        </v-card>

        <!-- Authenticated -->
        <v-card v-else>
          <v-card-title>
            <v-icon class="mr-2">mdi-account-group</v-icon>
            Authenticated Users
            <v-spacer></v-spacer>
            <v-chip color="success" text-color="white">
              <v-icon left small>mdi-account-check</v-icon>
              Authenticated
            </v-chip>
          </v-card-title>

          <v-card-text>
            <v-alert
              type="info"
              outlined
              class="mb-4"
            >
              <div class="text-h6 mb-2">Welcome, {{ currentUser.name }}!</div>
              <div class="text-caption">
                You are viewing all authenticated users. Only users who have connected their Solana wallets are shown here.
              </div>
            </v-alert>

            <v-data-table
              :headers="headers"
              :items="users"
              :loading="loading"
              class="elevation-1"
            >
              <template v-slot:item.actions="{ item }">
                <v-btn
                  small
                  color="primary"
                  @click="viewUser(item)"
                >
                  <v-icon small>mdi-eye</v-icon>
                </v-btn>
              </template>
            </v-data-table>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>


    <!-- User Details Dialog -->
    <v-dialog v-model="showUserDialog" max-width="500px">
      <v-card>
        <v-card-title>
          <span class="text-h5">User Details</span>
        </v-card-title>

        <v-card-text v-if="selectedUser">
          <v-container>
            <v-row>
              <v-col cols="12">
                <v-text-field
                  v-model="selectedUser.id"
                  label="ID"
                  readonly
                ></v-text-field>
              </v-col>
              <v-col cols="12">
                <v-text-field
                  v-model="selectedUser.name"
                  label="Name"
                  readonly
                ></v-text-field>
              </v-col>
              <v-col cols="12">
                <v-text-field
                  v-model="selectedUser.email"
                  label="Email"
                  readonly
                ></v-text-field>
              </v-col>
            </v-row>
          </v-container>
        </v-card-text>

        <v-card-actions>
          <v-spacer></v-spacer>
          <v-btn
            color="blue darken-1"
            text
            @click="showUserDialog = false"
          >
            Close
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Snackbar for notifications -->
    <v-snackbar
      v-model="snackbar"
      :color="snackbarColor"
      :timeout="3000"
    >
      {{ snackbarText }}
      <template v-slot:action="{ attrs }">
        <v-btn
          color="white"
          text
          v-bind="attrs"
          @click="snackbar = false"
        >
          Close
        </v-btn>
      </template>
    </v-snackbar>
  </v-container>
</template>

<script>
import { userService } from '@/services/api'

export default {
  name: 'Users',
  data() {
    return {
      users: [],
      loading: false,
      showUserDialog: false,
      selectedUser: null,
      snackbar: false,
      snackbarText: '',
      snackbarColor: 'success',
      isAuthenticated: false,
      currentUser: null,
      userBalance: '0.0000',
      headers: [
        { text: 'ID', value: 'id', sortable: true },
        { text: 'Name', value: 'name', sortable: true },
        { text: 'Email', value: 'email', sortable: true },
        { text: 'Wallet Address', value: 'walletAddress', sortable: true },
        { text: 'SOL Balance', value: 'solBalance', sortable: true },
        { text: 'Last Login', value: 'lastLogin', sortable: true },
        { text: 'Actions', value: 'actions', sortable: false }
      ]
    }
  },
  async mounted() {
    this.checkAuthenticationStatus()
    if (this.isAuthenticated) {
      await this.loadUsers()
    }
    
    // Listen for storage changes (when user logs out in another tab)
    window.addEventListener('storage', this.handleStorageChange)
  },
  
  beforeDestroy() {
    window.removeEventListener('storage', this.handleStorageChange)
  },
  methods: {
    checkAuthenticationStatus() {
      const isAuth = localStorage.getItem('isAuthenticated') === 'true'
      const userData = localStorage.getItem('solanaUser')
      
      this.isAuthenticated = isAuth
      
      if (isAuth && userData) {
        try {
          this.currentUser = JSON.parse(userData)
          this.userBalance = this.currentUser.solBalance ? this.currentUser.solBalance.toFixed(4) : '0.0000'
        } catch (error) {
          console.error('Error parsing user data:', error)
          this.logout()
        }
      }
    },

    async loadUsers() {
      this.loading = true
      try {
        this.users = await userService.getUsers()
      } catch (error) {
        this.showSnackbar('Failed to load users: ' + error.message, 'error')
        if (error.message.includes('not authenticated')) {
          this.logout()
        }
      } finally {
        this.loading = false
      }
    },

    viewUser(user) {
      this.selectedUser = { ...user }
      this.showUserDialog = true
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
      this.currentUser = null
      this.userBalance = '0.0000'
      
      // Refresh page to clear all cached data
      window.location.reload()
    },

    showSnackbar(text, color = 'success') {
      this.snackbarText = text
      this.snackbarColor = color
      this.snackbar = true
    },

    handleStorageChange(event) {
      // Handle storage changes from other tabs
      if (event.key === 'isAuthenticated' || event.key === 'solanaUser') {
        this.checkAuthenticationStatus()
        if (this.isAuthenticated) {
          this.loadUsers()
        }
      }
    }
  }
}
</script>
