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
import i18next from 'i18next'
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import { formatDecimal, formatNumber, formatTimestampToDate } from '../format'

beforeEach(async () => {
  await i18next.changeLanguage('tr')
})

afterEach(async () => {
  await i18next.changeLanguage('en')
})

describe('Turkish number notation', () => {
  test('uses a dot for thousands and a comma for decimals', () => {
    expect(formatNumber(1234.56)).toBe('1.234,56')
    expect(formatNumber(1234567)).toBe('1.234.567')
  })

  test('cuts long float tails', () => {
    expect(formatNumber(0.09930800000000001)).toBe('0,1')
    expect(formatDecimal(0.8, 2)).toBe('0,8')
    expect(formatDecimal(0.09930800000000001, 4)).toBe('0,0993')
  })

  test('keeps minimum decimals when asked', () => {
    expect(formatDecimal(245.02, 2, 2)).toBe('245,02')
    expect(formatDecimal(0, 2, 2)).toBe('0,00')
  })

  test('shows a dash for missing values', () => {
    expect(formatNumber(null)).toBe('-')
    expect(formatDecimal(Number.NaN, 2)).toBe('-')
  })

  test('follows another interface language when it is not Turkish', async () => {
    await i18next.changeLanguage('en')
    expect(formatNumber(1234.56)).toBe('1,234.56')
  })
})

describe('timestamp notation', () => {
  test('formats a timestamp as dd.MM.yyyy HH:mm:ss in Istanbul time', () => {
    const ts = Date.parse('2026-10-23T21:30:00Z') / 1000
    expect(formatTimestampToDate(ts)).toBe('24.10.2026 00:30:00')
  })
})
