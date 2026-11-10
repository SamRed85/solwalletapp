import '@mdi/font/css/materialdesignicons.css'
import 'vuetify/styles'
import { createVuetify } from 'vuetify'

const savedTheme = localStorage.getItem('theme')

// One indigo accent on flat surfaces. Dark is the default. The same accent
// is used in light, with the surfaces inverted.
const light = {
  primary: '#5B6CFF',
  secondary: '#E6E7EE',
  accent: '#5B6CFF',
  error: '#F04452',
  info: '#5B6CFF',
  success: '#16A34A',
  warning: '#D99A00',
  background: '#F4F5F8',
  surface: '#FFFFFF',
  'surface-variant': '#EDEEF3',
  'on-background': '#12121A',
  'on-surface': '#12121A',
  'on-primary': '#FFFFFF',
  'on-secondary': '#12121A'
}

const dark = {
  primary: '#5B6CFF',
  secondary: '#1E1E2A',
  accent: '#5B6CFF',
  error: '#F04452',
  info: '#5B6CFF',
  success: '#22C55E',
  warning: '#F5B800',
  background: '#0B0B12',
  surface: '#15151F',
  'surface-variant': '#1E1E2A',
  'on-background': '#F5F5F7',
  'on-surface': '#F5F5F7',
  'on-primary': '#FFFFFF',
  'on-secondary': '#F5F5F7'
}

export default createVuetify({
  theme: {
    defaultTheme: savedTheme === 'light' ? 'light' : 'dark',
    themes: {
      light: { colors: light },
      dark: { dark: true, colors: dark }
    }
  }
})
