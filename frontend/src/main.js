import { createApp } from 'vue'
import '@fontsource/geist/400.css'
import '@fontsource/geist/500.css'
import '@fontsource/geist/600.css'
import '@fontsource/geist/700.css'
import '@fontsource/geist/800.css'
import '@fontsource/geist-mono/400.css'
import App from './App.vue'
import router from './router'
import vuetify from './plugins/vuetify'
import './styles/type.css'
import './styles/surface.css'

// On a phone, Phantom only injects its provider inside its own browser. It can arrive after this script.
function markWalletBrowser () {
  const phone = /Mobi|Android|iPhone|iPad/i.test(navigator.userAgent)
  if (/Phantom|Solflare|Backpack/i.test(navigator.userAgent) || (phone && window.phantom?.solana?.isPhantom)) {
    document.documentElement.classList.add('in-wallet')
  }
}
markWalletBrowser()
window.addEventListener('load', markWalletBrowser, { once: true })

createApp(App).use(router).use(vuetify).mount('#app')
