import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import router from './router'
import { useAuthStore } from './stores/auth'
import './assets/main.css'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)

const auth = useAuthStore(pinia)
await auth.restoreSession()

window.addEventListener('shorturl:auth-expired', () => {
  auth.clearLocalSession()
  if (router.currentRoute.value.name !== 'login') {
    void router.replace({ name: 'login', query: { expired: '1' } })
  }
})

app.use(router)
await router.isReady()
app.mount('#app')
