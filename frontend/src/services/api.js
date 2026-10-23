import axios from 'axios'
import { loadSession } from './session'

const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '',
  timeout: 30000
})

let onUnauthorized = () => {}

export function setUnauthorizedHandler (fn) {
  onUnauthorized = fn
}

api.interceptors.request.use(config => {
  const session = loadSession()
  if (session) config.headers.Authorization = `Bearer ${session.token}`
  return config
})

api.interceptors.response.use(
  response => response,
  error => {
    if (error.response?.status === 401 && error.config?.headers?.Authorization) {
      onUnauthorized()
    }
    return Promise.reject(new Error(error.response?.data?.error || error.message))
  }
)

const data = promise => promise.then(res => res.data)

export const healthApi = {
  check: () => data(api.get('/health'))
}

export const configApi = {
  get: () => data(api.get('/api/config'))
}

export const authApi = {
  nonce: publicKey => data(api.post('/api/auth/nonce', { publicKey })),
  login: (publicKey, nonce, signature) => data(api.post('/api/auth/login', { publicKey, nonce, signature })),
  me: () => data(api.get('/api/me')),
  updateProfile: displayName => data(api.put('/api/me', { displayName }))
}

export const userApi = {
  list: () => data(api.get('/api/users')),
  update: (wallet, displayName) => data(api.put(`/api/users/${wallet}`, { displayName })),
  remove: wallet => api.delete(`/api/users/${wallet}`)
}

export const eventApi = {
  list: () => data(api.get('/api/events')),
  get: id => data(api.get(`/api/events/${id}`)),
  create: event => data(api.post('/api/events', event)),
  setGames: (id, games) => data(api.put(`/api/events/${id}/games`, { games })),
  setChain: (id, chain) => data(api.put(`/api/events/${id}/chain`, chain)),
  syncChain: id => data(api.post(`/api/events/${id}/chain/sync`)),
  // Drawing cancelled games searches devnet blocks, which can take a while on the public RPC.
  setResults: (id, results) => data(api.post(`/api/events/${id}/results`, { results }, { timeout: 150000 })),
  payouts: id => data(api.get(`/api/events/${id}/payouts`))
}

export const entryApi = {
  create: (eventId, system, signature) => data(api.post(`/api/events/${eventId}/entries`, { system, signature })),
  recover: (eventId, system, signature) => data(api.post(`/api/events/${eventId}/payment-recoveries`, { system, signature })),
  mine: () => data(api.get('/api/entries'))
}

export const recoveryApi = {
  list: () => data(api.get('/api/payment-recoveries')),
  refund: (signature, refundSignature) => data(api.put(`/api/payment-recoveries/${signature}`, { refundSignature }))
}
