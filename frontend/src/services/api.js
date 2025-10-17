import axios from 'axios'

const API_BASE_URL = process.env.NODE_ENV === 'production' 
  ? 'https://your-backend-url.com' 
  : 'http://localhost:8080'

const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// Request interceptor
api.interceptors.request.use(
  config => {
    console.log(`Making ${config.method.toUpperCase()} request to ${config.url}`)
    return config
  },
  error => {
    return Promise.reject(error)
  }
)

// Response interceptor
api.interceptors.response.use(
  response => {
    return response
  },
  error => {
    console.error('API Error:', error.response?.data || error.message)
    return Promise.reject(error)
  }
)

export const userService = {
  // Get all users (requires authentication)
  async getUsers() {
    try {
      const userData = localStorage.getItem('solanaUser')
      if (!userData) {
        throw new Error('User not authenticated')
      }
      
      const user = JSON.parse(userData)
      const response = await api.get('/api/users', {
        headers: {
          'X-Wallet-Address': user.walletAddress
        }
      })
      return response.data
    } catch (error) {
      throw new Error(`Failed to fetch users: ${error.message}`)
    }
  },

  // Get user by ID (requires authentication)
  async getUserById(id) {
    try {
      const userData = localStorage.getItem('solanaUser')
      if (!userData) {
        throw new Error('User not authenticated')
      }
      
      const user = JSON.parse(userData)
      const response = await api.get(`/api/users/${id}`, {
        headers: {
          'X-Wallet-Address': user.walletAddress
        }
      })
      return response.data
    } catch (error) {
      throw new Error(`Failed to fetch user: ${error.message}`)
    }
  }
}

export const healthService = {
  // Check backend health
  async checkHealth() {
    try {
      const response = await api.get('/health')
      return response.data
    } catch (error) {
      throw new Error(`Health check failed: ${error.message}`)
    }
  }
}

export const authService = {
  // Authenticate with Solana wallet
  async authenticateWithSolana(authData) {
    try {
      const response = await api.post('/api/auth/solana', authData)
      return response.data
    } catch (error) {
      throw new Error(`Authentication failed: ${error.message}`)
    }
  }
}

export const adminService = {
  // Check if user is admin
  async checkAdminStatus() {
    try {
      const userData = localStorage.getItem('solanaUser')
      if (!userData) {
        throw new Error('User not authenticated')
      }
      
      const user = JSON.parse(userData)
      const response = await api.get('/api/admin/check', {
        headers: {
          'X-Wallet-Address': user.walletAddress
        }
      })
      return response.data
    } catch (error) {
      throw new Error(`Admin check failed: ${error.message}`)
    }
  }
}

export default api
