import { globalIgnores } from 'eslint/config'
import { defineConfigWithVueTs, vueTsConfigs } from '@vue/eslint-config-typescript'
import pluginVue from 'eslint-plugin-vue'
import skipFormatting from '@vue/eslint-config-prettier/skip-formatting'

export default defineConfigWithVueTs(
  {
    name: 'nexora/files-to-lint',
    files: ['**/*.{ts,mts,tsx,vue}'],
  },
  globalIgnores(['dist/**', 'coverage/**', 'src/api/schema.d.ts']),
  pluginVue.configs['flat/essential'],
  vueTsConfigs.recommended,
  {
    name: 'nexora/rules',
    rules: {
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
      'vue/component-name-in-template-casing': ['error', 'PascalCase'],
      'vue/no-unused-properties': 'off',
    },
  },
  skipFormatting,
)
