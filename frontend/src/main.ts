import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
import { applyTheme, effectiveTheme } from './settings.mjs'
applyTheme(effectiveTheme('system', window.matchMedia('(prefers-color-scheme: dark)').matches))
createApp(App).mount('#app')
