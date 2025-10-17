<template>
  <v-container>
    <v-row justify="center">
      <v-col cols="12" md="10">
        <v-card class="mx-auto">
          <v-card-title class="text-h4 text-center pa-6">
            <v-icon large color="warning" class="mr-3">mdi-shield-crown</v-icon>
            Admin Dashboard
          </v-card-title>
          
          <v-card-text class="pa-6">
            <v-alert
              type="warning"
              outlined
              class="mb-4"
            >
              <div class="text-h6">
                <v-icon left>mdi-shield-check</v-icon>
                Admin Access Granted
              </div>
              <div class="text-caption mt-2">
                You have administrative privileges for this application.
              </div>
            </v-alert>

            <v-row>
              <v-col cols="12" md="6">
                <v-card outlined>
                  <v-card-title>
                    <v-icon color="primary" class="mr-2">mdi-account-group</v-icon>
                    User Management
                  </v-card-title>
                  <v-card-text>
                    View and manage all authenticated users
                  </v-card-text>
                  <v-card-actions>
                    <v-btn color="primary" text @click="$router.push('/users')">
                      Manage Users
                      <v-icon right>mdi-arrow-right</v-icon>
                    </v-btn>
                  </v-card-actions>
                </v-card>
              </v-col>
              
              <v-col cols="12" md="6">
                <v-card outlined>
                  <v-card-title>
                    <v-icon color="success" class="mr-2">mdi-database</v-icon>
                    Database Stats
                  </v-card-title>
                  <v-card-text>
                    View database statistics and health
                  </v-card-text>
                  <v-card-actions>
                    <v-btn color="success" text disabled>
                      Coming Soon
                      <v-icon right>mdi-clock-outline</v-icon>
                    </v-btn>
                  </v-card-actions>
                </v-card>
              </v-col>
            </v-row>

            <v-row class="mt-4">
              <v-col cols="12" md="6">
                <v-card outlined>
                  <v-card-title>
                    <v-icon color="info" class="mr-2">mdi-cog</v-icon>
                    System Settings
                  </v-card-title>
                  <v-card-text>
                    Configure system-wide settings
                  </v-card-text>
                  <v-card-actions>
                    <v-btn color="info" text disabled>
                      Coming Soon
                      <v-icon right>mdi-clock-outline</v-icon>
                    </v-btn>
                  </v-card-actions>
                </v-card>
              </v-col>
              
              <v-col cols="12" md="6">
                <v-card outlined>
                  <v-card-title>
                    <v-icon color="error" class="mr-2">mdi-security</v-icon>
                    Security Logs
                  </v-card-title>
                  <v-card-text>
                    Monitor security events and access logs
                  </v-card-text>
                  <v-card-actions>
                    <v-btn color="error" text disabled>
                      Coming Soon
                      <v-icon right>mdi-clock-outline</v-icon>
                    </v-btn>
                  </v-card-actions>
                </v-card>
              </v-col>
            </v-row>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </v-container>
</template>

<script>
export default {
  name: 'Admin',
  data() {
    return {
      // Admin-specific data can be added here
    }
  },
  mounted() {
    // Verify admin status on component mount
    this.verifyAdminStatus()
  },
  methods: {
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

    async verifyAdminStatus() {
      try {
        const { adminService } = await import('@/services/api')
        const adminStatus = await adminService.checkAdminStatus()
        
        if (!adminStatus.isAdmin) {
          this.safeNavigate('/')
        }
      } catch (error) {
        console.error('Admin verification failed:', error)
        this.safeNavigate('/')
      }
    }
  }
}
</script>
