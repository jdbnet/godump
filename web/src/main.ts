import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { applyTheme, readInitialTheme } from './stores/theme'
import './styles/theme.css'

applyTheme(readInitialTheme())

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
