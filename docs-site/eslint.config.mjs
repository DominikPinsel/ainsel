import js from '@eslint/js'
import tseslint from 'typescript-eslint'
import reactHooks from 'eslint-plugin-react-hooks'

// Flat config, mirroring frontend/eslint.config.mjs. ESLint 9 dropped
// .eslintrc.* support and the --ext flag; file selection now lives in the
// `files` globs below and in the plugins' own configs.
//
// Note there is no `globals` block: tseslint.configs.recommended turns off
// no-undef (TypeScript already reports undeclared names), so browser and
// vitest globals need no declaration.
export default [
  js.configs.recommended,
  ...tseslint.configs.recommended,
  reactHooks.configs.flat.recommended,
  {
    files: ['src/**/*.ts', 'src/**/*.tsx'],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
    },
    rules: {
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
      'react-hooks/refs': 'off',
      'react-hooks/set-state-in-effect': 'off',
      'react-hooks/incompatible-library': 'off',
    },
  },
]
