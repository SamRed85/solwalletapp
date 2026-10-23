export const shortAddress = address => address ? `${address.slice(0, 4)}...${address.slice(-4)}` : ''

export const formatDate = value => value ? new Date(value).toLocaleString() : '—'

export const lamportsToSol = lamports => lamports / 1e9

export const formatSol = lamports => `${Number((lamports / 1e9).toFixed(9))} SOL`

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
