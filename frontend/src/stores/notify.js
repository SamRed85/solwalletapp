import { reactive } from 'vue'

export const snackbar = reactive({ show: false, text: '', color: 'success' })

export function notify (text, color = 'success') {
  Object.assign(snackbar, { show: true, text, color })
}

export const notifyError = error => notify(error?.message || String(error), 'error')
