import fs from 'node:fs'
import path from 'node:path'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'

// GitHub Pages serves project sites from a subpath named after the
// repository: https://<user>.github.io/ainsel/. The Vite base path must
// match that prefix. Override via env var when hosting elsewhere.
const basePath = process.env.DOCS_SITE_BASE_PATH ?? '/ainsel/'

// The docs site serves the same repository `docs/` directory as the in-app
// Docs page (docsify-style: `_sidebar.md` drives the navigation, each page
// is fetched on demand). This plugin makes that directory available to the
// Vite dev server and copies it into the build output so GitHub Pages can
// serve it as static assets — keeping a single source of truth for docs.
function repoDocsPlugin(base: string): Plugin {
  const docsDir = path.resolve(process.cwd(), '../docs')
  const escaped = base.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const prefix = new RegExp(`^${escaped}docs/(.+)$`)

  return {
    name: 'ainsel-docs-site-repo-docs',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const url = req.url || ''
        const m = url.match(prefix)
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

export default defineConfig({
  base: basePath,
  plugins: [react(), repoDocsPlugin(basePath)],
  server: {
    // 5173 is taken by the frontend dev server.
    port: 5174,
  },
})
