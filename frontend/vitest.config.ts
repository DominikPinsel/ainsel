import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    // A spec whose async utils give up must still report the failed assertion.
    // With the 3 s async budget in `src/test/setup.ts`, a spec with several
    // waits can outlast the 5 s default and would be reported as a bare test
    // timeout, which hides the actual element error.
    testTimeout: 10000,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html', 'lcov'],
      thresholds: {
        lines: 70,
        statements: 70,
        functions: 67,
        branches: 60,
      },
    },
  },
})