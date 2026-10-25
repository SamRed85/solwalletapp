import axios from 'axios'

const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '',
  timeout: 30000,
  withCredentials: true
})

let onUnauthorized = () => {}

export function setUnauthorizedHandler (fn) {
  onUnauthorized = fn
}

api.interceptors.response.use(
  response => response,
  error => {
    const url = error.config?.url || ''
    if (error.response?.status === 401 && !url.includes('/api/auth/')) {
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
  logout: () => api.post('/api/auth/logout'),
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
  remove: id => api.delete(`/api/events/${id}`),
  setGames: (id, games) => data(api.put(`/api/events/${id}/games`, { games })),
  setChain: (id, chain) => data(api.put(`/api/events/${id}/chain`, chain)),
  syncChain: id => data(api.post(`/api/events/${id}/chain/sync`)),
  // Drawing cancelled games searches devnet blocks, which can take a while on the public RPC.
  setResults: (id, results) => data(api.post(`/api/events/${id}/results`, { results }, { timeout: 150000 })),
  payouts: id => data(api.get(`/api/events/${id}/payouts`)),
  projection: id => data(api.get(`/api/events/${id}/projection`)),
  refreshScores: id => data(api.post(`/api/events/${id}/projection/refresh`, null, { timeout: 45000 })),
  searchFixtures: (team, date, away, timezone) => data(api.get('/api/football/fixtures', { params: { team, date, away, timezone } }))
}

export const revealApi = {
  arm: (coupon, ciphertext) => data(api.post('/api/reveals', { coupon, ciphertext }))
}

export const entryApi = {
  create: (eventId, system, signature) => data(api.post(`/api/events/${eventId}/entries`, { system, signature })),
  recover: (eventId, system, signature) => data(api.post(`/api/events/${eventId}/payment-recoveries`, { system, signature })),
  mine: () => data(api.get('/api/entries'))
}

export const lotteryApi = {
  get: () => data(api.get('/api/lottery'))
}

export const slipApi = {
  get: event => data(api.get('/api/slip-archive', { params: { event } })),
  ingest: signature => data(api.post('/api/slip-archive', { signature }))
}

export const recoveryApi = {
  list: () => data(api.get('/api/payment-recoveries')),
  refund: (signature, refundSignature) => data(api.put(`/api/payment-recoveries/${signature}`, { refundSignature }))
}
