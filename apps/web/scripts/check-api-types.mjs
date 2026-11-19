import { readFile } from 'node:fs/promises'

import openapiTS, { astToString, COMMENT_HEADER } from 'openapi-typescript'

const generated =
  COMMENT_HEADER +
  astToString(await openapiTS(new URL('../../../docs/api/openapi.yaml', import.meta.url)))
const committed = await readFile(new URL('../src/api/schema.d.ts', import.meta.url), 'utf8')

if (generated !== committed) {
  console.error('OpenAPI types differ from the contract. Run pnpm api:types and review the change.')
  const expectedLines = generated.split('\n')
  const actualLines = committed.split('\n')
  const max = Math.max(expectedLines.length, actualLines.length)
  let difference = 0
  while (difference < max && expectedLines[difference] === actualLines[difference]) difference++
  console.error('First differing line:', difference + 1)
  for (let index = Math.max(0, difference - 2); index <= Math.min(max - 1, difference + 8); index++) {
    console.error(`${index + 1} expected: ${expectedLines[index] ?? '<EOF>'}`)
    console.error(`${index + 1} actual:   ${actualLines[index] ?? '<EOF>'}`)
  }
  process.exitCode = 1
} else {
  console.log('OpenAPI types match the contract')
}
