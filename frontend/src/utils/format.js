import { display } from '@/stores/currency'

export const shortAddress = address => address ? `${address.slice(0, 4)}...${address.slice(-4)}` : ''

export const formatDate = value => value ? new Date(value).toLocaleString() : '—'

export const lamportsToSol = lamports => Number(lamports) / 1e9

const solText = sol => `${Number(sol.toFixed(9))} SOL`

const fiatText = (amount, currency) => {
  const abs = Math.abs(amount)
  const digits = abs !== 0 && abs < 0.01 ? 4 : 2
  return new Intl.NumberFormat(currency === 'SEK' ? 'sv-SE' : 'en-US', {
    style: 'currency',
    currency,
    minimumFractionDigits: digits,
    maximumFractionDigits: digits
  }).format(amount)
}

// Screen label only. Payments and settlement stay in SOL.
export const formatSol = lamports => {
  const sol = lamportsToSol(lamports)
  if (!Number.isFinite(sol)) return '—'
  const code = display.currency
  const perSol = code === 'USD' ? display.rates?.usd : code === 'SEK' ? display.rates?.sek : null
  if (!perSol) return solText(sol)
  return fiatText(sol * perSol, code)
}

export const explorerTx = signature => `https://explorer.solana.com/tx/${signature}?cluster=devnet`

export const explorerBlock = slot => `https://explorer.solana.com/block/${slot}?cluster=devnet`

// Value for <input type="datetime-local"> in the browser's time zone.
export const toLocalInput = value => {
  const d = new Date(value)
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
}

export const statusColors = {
  draft: 'grey',
  open: 'success',
  closed: 'warning',
  completed: 'primary',
  cancelled: 'error'
}

export const explorerAddress = address => `https://explorer.solana.com/address/${address}?cluster=devnet`

export const outcomes = [
  { value: '1', label: 'Home win' },
  { value: 'X', label: 'Draw' },
  { value: '2', label: 'Away win' }
]

export const outcomeLabels = Object.fromEntries(outcomes.map(o => [o.value, o.label]))
