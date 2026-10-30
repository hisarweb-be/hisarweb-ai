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
import { afterEach, describe, expect, it } from 'vitest'

import { formatDecimal } from '@/lib/format'

import { formatLatency, formatThroughput, formatUptimePct } from '../lib/format'

afterEach(async () => {
  await i18next.changeLanguage('en')
})

async function useLanguage(code: 'tr' | 'en') {
  i18next.addResourceBundle(code, 'translation', {}, true)
  await i18next.changeLanguage(code)
}

describe('performance figures follow the interface language', () => {
  it('shows a decimal comma in Turkish, for seconds and percentages alike', async () => {
    await useLanguage('tr')

    expect(formatLatency(7090)).toBe('7,09s')
    expect(formatUptimePct(100)).toBe('100,00%')
    expect(formatUptimePct(99.01)).toBe('99,01%')
    expect(formatThroughput(5)).toBe('5,00 t/s')
    expect(formatThroughput(42.5)).toBe('42,5 t/s')
    expect(formatThroughput(2500)).toBe('2,5K t/s')
  })

  it('shows a decimal point in English', async () => {
    await useLanguage('en')

    expect(formatLatency(7090)).toBe('7.09s')
    expect(formatUptimePct(100)).toBe('100.00%')
    expect(formatThroughput(5)).toBe('5.00 t/s')
    expect(formatThroughput(2500)).toBe('2.5K t/s')
  })

  it('uses the same notation as the success rate on the model card', async () => {
    await useLanguage('tr')

    // model-perf-badge.tsx writes the success rate with formatDecimal; the
    // latency next to it has to agree with that notation.
    expect(formatUptimePct(100)).toBe(`${formatDecimal(100, 2, 2)}%`)
    expect(formatLatency(7090).replace('s', '')).toBe(formatDecimal(7.09, 2, 2))
  })

  it('keeps milliseconds whole and unknown values as a dash', () => {
    expect(formatLatency(840.4)).toBe('840ms')
    expect(formatLatency(0)).toBe('—')
    expect(formatLatency(Number.NaN)).toBe('—')
    expect(formatUptimePct(Number.NaN)).toBe('—')
    expect(formatThroughput(0)).toBe('—')
  })
})
