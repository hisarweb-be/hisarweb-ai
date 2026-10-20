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

import { fillHwText, hwTexts, pickHwLanguage } from '../texts'

describe('HisarWeb page texts', () => {
  it('keeps every Turkish key in English as well', () => {
    expect(Object.keys(hwTexts.en).sort()).toEqual(
      Object.keys(hwTexts.tr).sort()
    )
  })

  it('reads Turkish unless the interface language is explicitly English', () => {
    expect(pickHwLanguage('tr')).toBe('tr')
    expect(pickHwLanguage('en-US')).toBe('en')
    expect(pickHwLanguage('fr')).toBe('tr')
    expect(pickHwLanguage(undefined)).toBe('tr')
  })

  it('names no upstream supplier and claims no parity with an original', () => {
    const all = JSON.stringify(hwTexts).toLowerCase()
    expect(all).not.toContain('lumia')
    expect(all).not.toContain('openrouter')
    expect(all).not.toContain('orijinal')
  })

  it('fills a placeholder and leaves an unknown one alone', () => {
    expect(fillHwText('{ms} ms', { ms: 1200 })).toBe('1200 ms')
    expect(fillHwText('{ms} ms', {})).toBe('{ms} ms')
  })
})
