/*
Test for the static resource hints in index.html (HIS-7996): the browser must
start fetching the boot chunk and the stylesheet while it still parses the
HTML, instead of finding them only at the end of the deferred script tags.
*/
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

const html = readFileSync(
  resolve(import.meta.dirname, '../../../index.html'),
  'utf8'
)

describe('index.html resource hints', () => {
  it('preloads the entry stylesheet', () => {
    expect(html).toMatch(/<link[^>]+rel="preload"[^>]+href="\/static\/css\/index\.css"/)
  })

  it('preloads and modulepreloads the entry script', () => {
    expect(html).toMatch(/<link[^>]+rel="preload"[^>]+href="\/static\/js\/index\.js"/)
  })

  it('modulepreloads every chunk index.js imports before first paint', () => {
    // Mirrors the vendor chunks rsbuild emits ahead of index.js (see the build
    // log of HIS-7996); preloading them lets the module graph start at once.
    for (const chunk of ['vendor-tanstack', 'vendor-ui-primitives', 'lib-react']) {
      expect(html).toMatch(
        new RegExp(`<link[^>]+rel="modulepreload"[^>]+href="/static/js/${chunk}\\.js"`)
      )
    }
    expect(html).toMatch(/<link[^>]+rel="modulepreload"[^>]+href="\/static\/js\/index\.js"/)
  })
})
