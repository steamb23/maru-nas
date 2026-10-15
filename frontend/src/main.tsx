import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './i18n'
import './index.css'
import App from './App.tsx'
import { applyTheme, readThemeHint } from './appearance/dom'

// 서버 응답을 기다리지 않고 이 브라우저의 마지막 테마로 먼저 그린다(첫 그림 깜빡임 방지).
applyTheme(readThemeHint() ?? 'auto')

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
