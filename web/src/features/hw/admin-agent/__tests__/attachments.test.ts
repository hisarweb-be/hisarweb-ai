/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, it } from 'vitest'

import {
  filesFromClipboard,
  MAX_IMAGE_BYTES,
  MAX_TEXT_CHARS,
  readAttachment,
} from '../attachments'

describe('readAttachment', () => {
  it('reads a small image as a data URL', async () => {
    const file = new File([new Uint8Array([137, 80, 78, 71])], 'shot.png', {
      type: 'image/png',
    })
    const result = await readAttachment(file)
    expect(result).toMatchObject({ name: 'shot.png', mime: 'image/png' })
    expect((result as { data: string }).data).toMatch(
      /^data:image\/png;base64,/
    )
  })

  it('names a nameless pasted image "screenshot"', async () => {
    const file = new File([new Uint8Array([1])], '', { type: 'image/webp' })
    expect(await readAttachment(file)).toMatchObject({ name: 'screenshot' })
  })

  it('refuses an image over the size limit', async () => {
    const file = new File([new Uint8Array(MAX_IMAGE_BYTES + 1)], 'big.png', {
      type: 'image/png',
    })
    expect(await readAttachment(file)).toBe('size')
  })

  it('reads a text file as plain text, also by extension only', async () => {
    const byType = new File(['hello'], 'note', { type: 'text/plain' })
    expect(await readAttachment(byType)).toEqual({
      name: 'note',
      mime: 'text/plain',
      data: 'hello',
    })
    const byName = new File(['{"a":1}'], 'error.JSON', { type: '' })
    expect(await readAttachment(byName)).toMatchObject({
      mime: 'text/plain',
      data: '{"a":1}',
    })
  })

  it('refuses an empty or too long text file', async () => {
    const empty = new File(['  \n'], 'empty.txt', { type: 'text/plain' })
    expect(await readAttachment(empty)).toBe('size')
    const long = new File(['x'.repeat(MAX_TEXT_CHARS + 1)], 'long.log', {
      type: 'text/plain',
    })
    expect(await readAttachment(long)).toBe('size')
  })

  it('refuses a type it cannot send', async () => {
    const file = new File(['%PDF'], 'doc.pdf', { type: 'application/pdf' })
    expect(await readAttachment(file)).toBe('type')
    const svg = new File(['<svg/>'], 'a.svg', { type: 'image/svg+xml' })
    expect(await readAttachment(svg)).toBe('type')
  })
})

describe('filesFromClipboard', () => {
  const item = (kind: string, file: File | null) => ({
    kind,
    getAsFile: () => file,
  })

  it('returns nothing without clipboard data', () => {
    expect(filesFromClipboard(null)).toEqual([])
  })

  it('keeps the files and drops text items and empty files', () => {
    const png = new File([new Uint8Array([1])], 'a.png', { type: 'image/png' })
    const data = {
      items: [item('string', null), item('file', png), item('file', null)],
    } as unknown as DataTransfer
    expect(filesFromClipboard(data)).toEqual([png])
  })
})
