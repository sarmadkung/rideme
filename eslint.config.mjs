import js from '@eslint/js';
import tseslint from 'typescript-eslint';

/**
 * Shared flat config. Every package/app re-exports this from its own
 * eslint.config.mjs so `eslint .` resolves identically from any cwd.
 */
export default tseslint.config(
  {
    ignores: [
      '**/node_modules/**',
      '**/dist/**',
      '**/build/**',
      '**/.next/**',
      '**/.expo/**',
      '**/.turbo/**',
      '**/coverage/**',
      'services/api/**',
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    // Expo's build-time configuration runs in Node before the bundler exists:
    // CommonJS, with the Node globals the shipped code must never reach for.
    // Scoped to this filename so the exemption cannot spread into app source.
    files: ['**/app.config.js'],
    languageOptions: {
      sourceType: 'commonjs',
      globals: {
        require: 'readonly',
        module: 'writable',
        process: 'readonly',
        __dirname: 'readonly',
      },
    },
    rules: {
      '@typescript-eslint/no-require-imports': 'off',
    },
  },
  {
    rules: {
      '@typescript-eslint/no-unused-vars': [
        'error',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
      ],
      '@typescript-eslint/consistent-type-imports': 'error',
    },
  },
);
