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
  },

  // Get admin wallet address for payments
  async getAdminWalletAddress() {
    try {
      const userData = localStorage.getItem('solanaUser')
      if (!userData) {
        throw new Error('User not authenticated')
      }
      
      const user = JSON.parse(userData)
      const response = await api.get('/api/admin/wallet', {
        headers: {
          'X-Wallet-Address': user.walletAddress
        }
      })
      return response.data
    } catch (error) {
      throw new Error(`Failed to get admin wallet address: ${error.message}`)
    }
  }
}

export const eventService = {
  // Get all events
  async getEvents() {
    try {
      const userData = localStorage.getItem('solanaUser')
      if (!userData) {
        throw new Error('User not authenticated')
      }
      
      const user = JSON.parse(userData)
      const response = await api.get('/api/events', {
        headers: {
          'X-Wallet-Address': user.walletAddress
        }
      })
      return response.data
    } catch (error) {
      throw new Error(`Failed to fetch events: ${error.message}`)
    }
  },

  // Create new event
  async createEvent(eventData) {
    try {
      const userData = localStorage.getItem('solanaUser')
      if (!userData) {
        throw new Error('User not authenticated')
      }
      
      const user = JSON.parse(userData)
      const response = await api.post('/api/events', eventData, {
        headers: {
          'X-Wallet-Address': user.walletAddress
        }
      })
      return response.data
    } catch (error) {
      throw new Error(`Failed to create event: ${error.message}`)
    }
  },

  // Get event with games
  async getEvent(eventId) {
    try {
      const userData = localStorage.getItem('solanaUser')
      if (!userData) {
        throw new Error('User not authenticated')
      }
      
      const user = JSON.parse(userData)
      const response = await api.get(`/api/events/${eventId}`, {
        headers: {
          'X-Wallet-Address': user.walletAddress
        }
      })
      return response.data
    } catch (error) {
      throw new Error(`Failed to fetch event: ${error.message}`)
    }
  },

  // Create games for event
  async createGames(eventId, games) {
    try {
      const userData = localStorage.getItem('solanaUser')
      if (!userData) {
        throw new Error('User not authenticated')
      }
      
      const user = JSON.parse(userData)
      const response = await api.post('/api/games', {
        eventId,
        games
      }, {
        headers: {
          'X-Wallet-Address': user.walletAddress
        }
      })
      return response.data
    } catch (error) {
      throw new Error(`Failed to create games: ${error.message}`)
    }
  },

  // Set game outcome
  async setGameOutcome(gameId, outcome) {
    try {
      const userData = localStorage.getItem('solanaUser')
      if (!userData) {
        throw new Error('User not authenticated')
      }
      
      const user = JSON.parse(userData)
      const response = await api.put(`/api/games/${gameId}`, {
        outcome
      }, {
        headers: {
          'X-Wallet-Address': user.walletAddress
        }
      })
      return response.data
    } catch (error) {
      throw new Error(`Failed to set game outcome: ${error.message}`)
    }
  },

  // Set event outcomes
  async setEventOutcomes(eventId, gameOutcomes) {
    try {
      const userData = localStorage.getItem('solanaUser')
      if (!userData) {
        throw new Error('User not authenticated')
      }
      
      const user = JSON.parse(userData)
      const response = await api.post(`/api/events/${eventId}/outcomes`, {
        gameOutcomes
      }, {
        headers: {
          'X-Wallet-Address': user.walletAddress
        }
      })
      return response.data
    } catch (error) {
      throw new Error(`Failed to set event outcomes: ${error.message}`)
    }
  }
}

export const predictionService = {
  // Get user predictions for an event
  async getPredictions(eventId) {
    try {
      const userData = localStorage.getItem('solanaUser')
      if (!userData) {
        throw new Error('User not authenticated')
      }
      
      const user = JSON.parse(userData)
      const response = await api.get(`/api/predictions?eventId=${eventId}`, {
        headers: {
          'X-Wallet-Address': user.walletAddress
        }
      })
      return response.data
    } catch (error) {
      throw new Error(`Failed to fetch predictions: ${error.message}`)
    }
  },

  // Create or update prediction
  async createPrediction(eventId, gameId, outcomes) {
    try {
      const userData = localStorage.getItem('solanaUser')
      if (!userData) {
        throw new Error('User not authenticated')
      }
      
      const user = JSON.parse(userData)
      const response = await api.post('/api/predictions', {
        eventId,
        gameId,
        outcomes
      }, {
        headers: {
          'X-Wallet-Address': user.walletAddress
        }
      })
      return response.data
    } catch (error) {
      throw new Error(`Failed to create prediction: ${error.message}`)
    }
  },

  // Create coupon prediction (new method for coupon system)
  async createCouponPrediction(eventId, coupon, transactionHash) {
    try {
      const userData = localStorage.getItem('solanaUser')
      if (!userData) {
        throw new Error('User not authenticated')
      }
      
      const user = JSON.parse(userData)
      const response = await api.post('/api/predictions/coupon', {
        eventId,
        coupon,
        transactionHash
      }, {
        headers: {
          'X-Wallet-Address': user.walletAddress
        }
      })
      return response.data
    } catch (error) {
      throw new Error(`Failed to create coupon prediction: ${error.message}`)
    }
  },

  // Get all user predictions (for My Coupons view)
  async getAllUserPredictions() {
    try {
      const userData = localStorage.getItem('solanaUser')
      if (!userData) {
        throw new Error('User not authenticated')
      }
      
      const user = JSON.parse(userData)
      const response = await api.get('/api/predictions/all', {
        headers: {
          'X-Wallet-Address': user.walletAddress
        }
      })
      return response.data
    } catch (error) {
      throw new Error(`Failed to get user predictions: ${error.message}`)
    }
  }
}

export default api
