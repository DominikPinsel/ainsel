import fs from 'node:fs'
import path from 'node:path'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'

// The in-app Docs page browses the repository's own `docs/` directory at
// runtime (docsify-style): _sidebar.md drives the navigation and each
// `<slug>.md` is fetched on demand. This plugin makes that directory
// available to the Vite dev server and copies it into the build output so
// nginx serves it in production — no per-page code changes required to add
// a new doc.
//
//   dev:   GET /docs/<file>      → served from <repo-root>/docs/
//   build: dist/docs/            ← recursive copy of <repo-root>/docs/
function repoDocsPlugin(): Plugin {
  const docsDir = path.resolve(process.cwd(), '../docs')

  return {
    name: 'ainsel-repo-docs',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const url = req.url || ''
        // Match /docs/... with or without the build-time base placeholder.
        const m = url.match(/^\/(?:__BASE_PATH__\/)?docs\/(.+)$/)
        if (!m) return next()
        const rel = decodeURIComponent(m[1].split('?')[0])
        const filePath = path.join(docsDir, rel)
        if (!filePath.startsWith(docsDir)) return next()
        fs.readFile(filePath, (err, data) => {
          if (err) return next()
          res.setHeader('Content-Type', 'text/markdown; charset=utf-8')
          res.end(data)
        })
      })
    },
    closeBundle() {
      const distDocs = path.resolve(process.cwd(), 'dist/docs')
      if (fs.existsSync(docsDir)) {
        fs.cpSync(docsDir, distDocs, { recursive: true })
      }
    },
  }
}

// Base path is a placeholder at build time and gets rewritten at container
// startup by /docker-entrypoint.d/15-inject-base-path.sh based on the
// BASE_PATH env var. This way a single built image is deployable at any
// subpath (or the root) without rebuilding.
//
// The literal `/__BASE_PATH__/` appears in:
// - asset URLs in dist/index.html
// - the Vite runtime const exposed to app code as import.meta.env.BASE_URL
// - any dynamic imports
//
// At runtime the placeholder is sed-replaced everywhere with the real prefix
// (e.g. `/ainsel-dev/`) or with `/` when no BASE_PATH is set.
export default defineConfig({
  base: '/__BASE_PATH__/',
  plugins: [react(), repoDocsPlugin()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: process.env.VITE_API_TARGET ?? 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})