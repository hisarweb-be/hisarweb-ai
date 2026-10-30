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
import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  HW_LEGACY_CURRENCY_CONFIG,
  type HwCurrency,
  type HwCurrencyConfig,
} from '@/features/hw/lib/currency'
import { useHwCurrencyStore } from '@/stores/hw-currency-store'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { ModelCard } from '../components/model-card'
import { formatPrice } from '../lib/price'
import type { PricingModel } from '../types'

const TRY_PER_USD = 50
const EUR_PER_USD = 0.9

// Same shape the live server sends: three currencies, USD is always 1.
const threeCurrencies: HwCurrencyConfig = {
  currencies: ['TRY', 'EUR', 'USD'],
  rates: { TRY: TRY_PER_USD, EUR: EUR_PER_USD, USD: 1 },
  topUp: {},
}

// A model that costs 2 USD per 1M input tokens and 10 USD per 1M output tokens.
const model: PricingModel = {
  id: 1,
  model_name: 'example-model',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 5,
  enable_groups: ['default'],
  group_ratio: { default: 1 },
}

function pickCurrency(currency: HwCurrency) {
  useHwCurrencyStore.setState({ preferred: currency })
}

beforeEach(() => {
  // The console shows lira, as the live site does (display type CUSTOM).
  useSystemConfigStore.getState().setConfig({
    currency: {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'CUSTOM',
      customCurrencySymbol: '₺',
      customCurrencyExchangeRate: TRY_PER_USD,
    },
  })
  useHwCurrencyStore.setState({ config: threeCurrencies, preferred: null })
})

afterEach(() => {
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  useHwCurrencyStore.setState({
    config: HW_LEGACY_CURRENCY_CONFIG,
    preferred: null,
  })
  vi.unstubAllGlobals()
})

describe('pricing prices follow the currency switch', () => {
  it('shows lira when the visitor chose TRY', () => {
    pickCurrency('TRY')
    expect(formatPrice(model, 'input', 'M')).toBe('₺100.00')
    expect(formatPrice(model, 'output', 'M')).toBe('₺500.00')
  })

  it('shows euro, converted with the server EUR rate, when the visitor chose EUR', () => {
    pickCurrency('EUR')
    expect(formatPrice(model, 'input', 'M')).toBe('€1.80')
    expect(formatPrice(model, 'output', 'M')).toBe('€9.00')
  })

  it('shows dollars unchanged when the visitor chose USD', () => {
    pickCurrency('USD')
    expect(formatPrice(model, 'input', 'M')).toBe('$2.00')
    expect(formatPrice(model, 'output', 'M')).toBe('$10.00')
  })

  it('leaves out the symbol when the caller asks for the bare number', () => {
    pickCurrency('EUR')
    expect(
      formatPrice(model, 'input', 'M', false, 1, 1, undefined, false)
    ).toBe('1.80')
  })

  it('keeps the console display setting on an older server without currency data', () => {
    useHwCurrencyStore.setState({
      config: HW_LEGACY_CURRENCY_CONFIG,
      preferred: 'EUR',
    })
    expect(formatPrice(model, 'input', 'M')).toBe('₺100.00')
  })

  it('keeps the console display setting when the chosen currency has no rate', () => {
    useHwCurrencyStore.setState({
      config: {
        currencies: ['TRY', 'EUR'],
        rates: { TRY: TRY_PER_USD },
        topUp: {},
      },
      preferred: 'EUR',
    })
    // EUR is not sold without a rate, so the choice falls back to TRY.
    expect(formatPrice(model, 'input', 'M')).toBe('₺100.00')
  })

  it('re-renders a model card when the switch changes', () => {
    pickCurrency('TRY')
    render(<ModelCard model={model} onClick={vi.fn()} tokenUnit='M' />)
    expect(screen.getByText('₺100.00')).toBeVisible()
    expect(screen.queryByText('€1.80')).not.toBeInTheDocument()

    act(() => pickCurrency('EUR'))
    expect(screen.getByText('€1.80')).toBeVisible()
    expect(screen.getByText('€9.00')).toBeVisible()
    expect(screen.queryByText('₺100.00')).not.toBeInTheDocument()

    act(() => pickCurrency('USD'))
    expect(screen.getByText('$2.00')).toBeVisible()
    expect(screen.getByText('$10.00')).toBeVisible()
  })
})
