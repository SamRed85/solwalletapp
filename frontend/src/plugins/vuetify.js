import '@mdi/font/css/materialdesignicons.css'
import 'vuetify/styles'
import { createVuetify } from 'vuetify'

const savedTheme = localStorage.getItem('theme')

// Colors live here, the same way Qtagg declares them: one named set for light
// and one for dark. primary, secondary, and the status colors are what
// buttons and chips use. appbar, surface, background, and panel are the
// stops the page gradients are built from.
const light = {
  primary: '#1976D2',
  secondary: '#424242',
  accent: '#82B1FF',
  error: '#FF5252',
  info: '#2196F3',
  success: '#4CAF50',
  warning: '#FFC107',
  appbar: '#1565C0',
  appbarMid: '#1E88E5',
  appbarCenter: '#42A5F5',
  background: '#F7F9FC',
  backgroundEdge: '#D5E2F2',
  backgroundMid: '#E4ECF6',
  backgroundCenter: '#F7F9FC',
  surface: '#FFFFFF',
  surfaceEdge: '#E7EEF7',
  surfaceMid: '#F2F5FB',
  surfaceCenter: '#FFFFFF',
  panel: '#FFFFFF',
  panelMid: '#FFFFFF',
  panelCenter: '#FFFFFF'
}

const dark = {
  primary: '#2196F3',
  secondary: '#424242',
  accent: '#FF4081',
  error: '#FF5252',
  info: '#2196F3',
  success: '#4CAF50',
  warning: '#FFC107',
  appbar: '#0D47A1',
  appbarMid: '#1565C0',
  appbarCenter: '#1E88E5',
  background: '#0C1016',
  backgroundEdge: '#0C1016',
  backgroundMid: '#1A2838',
  backgroundCenter: '#2A3D56',
  surface: '#151B24',
  surfaceEdge: '#151B24',
  surfaceMid: '#243044',
  surfaceCenter: '#3D5270',
  panel: '#1E1E1E',
  panelMid: '#2A2A2A',
  panelCenter: '#3A4558'
}

export default createVuetify({
  theme: {
    defaultTheme: savedTheme === 'dark' ? 'dark' : 'light',
    themes: {
      light: { colors: light },
      dark: { dark: true, colors: dark }
    }
  }
})
