module.exports = {
  root: true,
  env: { browser: true, es2022: true },
  extends: [
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended',
    'plugin:react-hooks/recommended',
  ],
  parser: '@typescript-eslint/parser',
  parserOptions: { ecmaVersion: 'latest', sourceType: 'module' },
  plugins: ['react-refresh'],
  rules: {
    'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
    '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
    // eslint-plugin-react-hooks v7 adds React Compiler rules. The compiler is
    // not enabled in this build; the existing patterns flagged below are
    // intentional (URL-sync effects, react-hook-form integration, ref-based
    // row ids). Keep them off until the compiler migration is tackled.
    'react-hooks/refs': 'off',
    'react-hooks/set-state-in-effect': 'off',
    'react-hooks/incompatible-library': 'off',
  },
}
