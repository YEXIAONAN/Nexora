import { createPinia } from 'pinia'
import { createApp } from 'vue'

import App from './App.vue'
import router from './router'
import { useThemeStore } from './stores/theme'
import './styles/main.css'

const app = createApp(App)
app.use(createPinia())

useThemeStore().initialize()

app.use(router)
app.mount('#app')
