import ReactDOM from 'react-dom/client'
import { QueryClientProvider } from '@tanstack/react-query'
import '../i18n'
import { queryClient } from '@/lib/query-client'
import MinimalApp from './AppShell'
import '../App.css'
import './wails.css'

// Disable the browser context menu to prevent accidental reloads (parity
// with the Tauri shell).
document.addEventListener('contextmenu', e => {
  e.preventDefault()
})

/** Hide the initial loader spinner (parity with the Tauri shell). */
function hideAppLoader() {
  const loader = document.getElementById('app-loader')
  if (loader) {
    loader.classList.add('hidden')
    setTimeout(() => loader.remove(), 300)
  }
}

const rootElement = document.getElementById('root')
if (!rootElement) {
  throw new Error('Missing #root element')
}

ReactDOM.createRoot(rootElement).render(
  <QueryClientProvider client={queryClient}>
    <MinimalApp />
  </QueryClientProvider>
)

// The first frame is painted from the embedded HTML; drop the loader so the
// app underneath becomes visible immediately.
requestAnimationFrame(() => hideAppLoader())
