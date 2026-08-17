import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

// The published docs site renders with the exact same components and styles
// as the in-app Docs page, so both stay in sync by construction.
import '../../frontend/src/design/tokens.css'
import '../../frontend/src/design/fonts.css'
import '../../frontend/src/design/base.css'
import '../../frontend/src/design/primitives.css'
import { initTheme } from '../../frontend/src/theme'
import { App } from './App'

initTheme()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
