import { readFile } from 'node:fs/promises'

import openapiTS, { astToString, COMMENT_HEADER } from 'openapi-typescript'

const generated =
  COMMENT_HEADER +
  astToString(await openapiTS(new URL('../../../docs/api/openapi.yaml', import.meta.url)))
const committed = await readFile(new URL('../src/api/schema.d.ts', import.meta.url), 'utf8')

if (generated !== committed) {
  console.error('OpenAPI types differ from the contract. Run pnpm api:types and review the change.')
  process.exitCode = 1
} else {
  console.log('OpenAPI types match the contract')
}
