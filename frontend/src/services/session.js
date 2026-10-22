const KEY = 'session'

export function loadSession () {
  try {
    const session = JSON.parse(localStorage.getItem(KEY))
    if (session?.token && new Date(session.expiresAt) > new Date()) return session
  } catch {
    // corrupt value, fall through and clear it
  }
  localStorage.removeItem(KEY)
  return null
}

export function saveSession (session) {
  localStorage.setItem(KEY, JSON.stringify(session))
}

export function clearSession () {
  localStorage.removeItem(KEY)
}

export const SESSION_KEY = KEY
