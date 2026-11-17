import { createPinia } from 'pinia'
import { createApp } from 'vue'

import App from './App.vue'
import { SESSION_EXPIRED_EVENT } from './api/client'
import router from './router'
import { useAuthStore } from './stores/auth'
import { useTeamsStore } from './stores/teams'
import { useThemeStore } from './stores/theme'
import './styles/main.css'

const app = createApp(App)
app.use(createPinia())

useThemeStore().initialize()

window.addEventListener(SESSION_EXPIRED_EVENT, () => {
  const auth = useAuthStore()
  const wasAuthenticated = auth.isAuthenticated
  auth.expire()
  useTeamsStore().reset()
  if (wasAuthenticated && router.currentRoute.value.name !== 'login') {
    void router.push({ name: 'login', query: { expired: '1' } })
  }
})

app.use(router)
app.mount('#app')
