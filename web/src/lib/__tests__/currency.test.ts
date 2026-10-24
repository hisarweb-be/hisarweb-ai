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

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { formatCurrencyFromUSD } from '../currency'

beforeEach(async () => {
  await i18next.changeLanguage('tr')
})

afterEach(async () => {
  await i18next.changeLanguage('en')
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG },
  })
})

describe('money notation', () => {
  test('writes dollars with Turkish digits and two decimals', () => {
    expect(formatCurrencyFromUSD(12.5)).toBe('$12,50')
    expect(formatCurrencyFromUSD(0)).toBe('$0,00')
  })

  test('puts a thousands dot in larger amounts', () => {
    expect(formatCurrencyFromUSD(1234.5)).toBe('$1.234,50')
  })

  test('shows lira with the symbol first and no space', () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CUSTOM',
        customCurrencySymbol: 'TL',
        customCurrencyExchangeRate: 24.5,
      },
    })
    expect(formatCurrencyFromUSD(10)).toBe('₺245,00')
    expect(formatCurrencyFromUSD(0)).toBe('₺0,00')
    expect(formatCurrencyFromUSD(10.0008)).toBe('₺245,02')
  })
})
