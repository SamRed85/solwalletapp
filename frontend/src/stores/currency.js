import { reactive } from 'vue'

const STORAGE_KEY = 'sol.displayCurrency'
const saved = localStorage.getItem(STORAGE_KEY)

export const display = reactive({
  currency: saved === 'USD' || saved === 'SEK' ? saved : 'SOL',
  rates: null,
  rateError: false
})

export function setCurrency (code) {
  if (code !== 'SOL' && code !== 'USD' && code !== 'SEK') return
  display.currency = code
  localStorage.setItem(STORAGE_KEY, code)
}

export async function loadRates () {
  try {
    const response = await fetch('https://api.coingecko.com/api/v3/simple/price?ids=solana&vs_currencies=usd,sek')
    if (!response.ok) throw new Error('price lookup failed')
    const data = await response.json()
    const usd = Number(data?.solana?.usd)
    const sek = Number(data?.solana?.sek)
    if (usd > 0 && sek > 0) {
      display.rates = { usd, sek }
      display.rateError = false
      return
    }
  } catch {
    // Fall through to the backup quotes.
  }
  try {
    const [sol, fx] = await Promise.all([
      fetch('https://api.binance.com/api/v3/ticker/price?symbol=SOLUSDT').then(response => response.json()),
      fetch('https://api.frankfurter.app/latest?from=USD&to=SEK').then(response => response.json())
    ])
    const usd = Number(sol?.price)
    const sekPerUsd = Number(fx?.rates?.SEK)
    if (usd > 0 && sekPerUsd > 0) {
      display.rates = { usd, sek: usd * sekPerUsd }
      display.rateError = false
      return
    }
  } catch {
    // Leave the last good quote in place.
  }
  if (!display.rates) display.rateError = true
}
