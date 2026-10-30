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

  it('offers card payment only: Revolut is named as processor, never as a choice', () => {
    for (const language of ['tr', 'en'] as const) {
      const dictionary = hwTexts[language] as Record<string, string>
      const naming = Object.keys(dictionary).filter((key) =>
        /revolut/i.test(dictionary[key])
      )
      expect(naming).toEqual(['trustPayments'])
    }
  })

  it('gives every lira or KDV wording a variant for the other currencies', () => {
    const lira = {
      tr: /Türk lira|TL olarak|KDV dahil/,
      en: /Turkish lira|VAT/,
    }
    for (const language of ['tr', 'en'] as const) {
      const dictionary = hwTexts[language] as Record<string, string>
      const missing = Object.keys(dictionary).filter(
        (key) =>
          !key.endsWith('Foreign') &&
          !key.endsWith('TRY') &&
          lira[language].test(dictionary[key]) &&
          !(`${key}Foreign` in dictionary)
      )
      expect(missing).toEqual([])
    }
  })

  // Turkish said "her alışverişte" (every purchase) while English said "later
  // purchases"; the price without the welcome discount applies to every
  // purchase, so both languages name it that way.
  it('calls the standard price the price of every purchase in both languages', () => {
    expect(hwTexts.tr.compareOursRepeat).toBe(
      'HisarWeb AI fiyatı (her alışverişte)'
    )
    expect(hwTexts.en.compareOursRepeat).toBe(
      'HisarWeb AI price (every purchase)'
    )
    expect(hwTexts.tr.compareSavingRepeat).toBe(
      'Her alışverişte tasarruf: {amount}'
    )
    expect(hwTexts.en.compareSavingRepeat).toBe(
      'Saving on every purchase: {amount}'
    )
  })

  it('fills a placeholder and leaves an unknown one alone', () => {
    expect(fillHwText('{ms} ms', { ms: 1200 })).toBe('1200 ms')
    expect(fillHwText('{ms} ms', {})).toBe('{ms} ms')
  })
})
