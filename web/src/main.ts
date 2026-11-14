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
  const currentRoute = router.currentRoute.value
  auth.clearLocalSession()
  if (currentRoute.query.auth !== 'login') {
    const query: Record<string, string> = {
      auth: 'login',
      expired: '1',
    }
    if (currentRoute.meta.requiresAuth && currentRoute.fullPath !== '/') {
      query.redirect = currentRoute.fullPath
    }
    void router.replace({ name: 'try', query })
  }
})

app.use(router)
await router.isReady()
app.mount('#app')
