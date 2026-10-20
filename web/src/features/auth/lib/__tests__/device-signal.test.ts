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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Every PNG data URL ends with the same constant chunk, so this tail must not
// decide the token on its own.
const PNG_FIXED_TAIL =
  'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA='

function stubCanvasPixels(pixels: string): void {
  const createElement = document.createElement.bind(document)
  vi.spyOn(document, 'createElement').mockImplementation(((
    tagName: string
  ) => {
    if (tagName !== 'canvas') return createElement(tagName)
    const context = {
      textBaseline: '',
      font: '',
      fillStyle: '',
      fillRect: () => undefined,
      fillText: () => undefined,
    }
    return {
      getContext: () => context,
      toDataURL: () => `data:image/png;base64,${pixels}${PNG_FIXED_TAIL}`,
    } as unknown as HTMLElement
  }) as typeof document.createElement)
}

// Module state (the cached token) lives per import, so every case starts with a
// fresh module and an empty session storage.
async function freshToken(): Promise<string> {
  vi.resetModules()
  const { getHwDeviceToken } = await import('../device-signal')
  return getHwDeviceToken()
}

describe('getHwDeviceToken', () => {
  beforeEach(() => {
    window.sessionStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('returns the same 64-character hex token on every call', async () => {
    const first = await freshToken()
    const second = await freshToken()

    expect(first).toMatch(/^[0-9a-f]{64}$/)
    expect(second).toBe(first)
  })

  it('hashes the canvas pixels instead of the constant PNG tail', async () => {
    stubCanvasPixels('AAAA')
    const firstPixels = await freshToken()

    vi.restoreAllMocks()
    window.sessionStorage.clear()
    stubCanvasPixels('BBBB')
    const otherPixels = await freshToken()

    expect(otherPixels).not.toBe(firstPixels)
  })
})
