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
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useHwCurrencyStore } from '@/stores/hw-currency-store'

import { HwCurrencySwitch } from '../currency-switch'
import {
  HW_LEGACY_CURRENCY_CONFIG,
  parseHwCurrencyConfig,
} from '../../lib/currency'

const { put } = vi.hoisted(() => ({ put: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { put } }))

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

const config = parseHwCurrencyConfig({
  hw_currencies: ['TRY', 'EUR', 'USD'],
  hw_rates: { TRY: '50', EUR: '0.9', USD: '1' },
  hw_topup: {
    TRY: { min: 100, max: 20000, presets: [250] },
    EUR: { min: 3, max: 400, presets: [10] },
    USD: { min: 3, max: 450, presets: [10] },
  },
})

function renderSwitch() {
  return render(
    <I18nextProvider i18n={i18n}>
      <HwCurrencySwitch />
    </I18nextProvider>
  )
}

describe('HwCurrencySwitch', () => {
  beforeEach(() => {
    window.localStorage.clear()
    useHwCurrencyStore.setState({ config, preferred: 'TRY' })
  })
  afterEach(() => {
    cleanup()
    put.mockReset()
  })

  it('offers the currencies on sale and marks the active one', () => {
    renderSwitch()
    const group = screen.getByRole('group', { name: 'Currency' })
    expect(group.querySelectorAll('button')).toHaveLength(3)
    expect(screen.getByRole('button', { name: /Turkish lira/ })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
  })

  it('switches currency and remembers it in this browser', () => {
    renderSwitch()
    fireEvent.click(screen.getByRole('button', { name: /Euro/ }))
    expect(useHwCurrencyStore.getState().preferred).toBe('EUR')
    expect(window.localStorage.getItem('hw-currency')).toBe('EUR')
    expect(put).not.toHaveBeenCalled()
  })

  it('renders nothing when only one currency is on sale', () => {
    useHwCurrencyStore.setState({ config: HW_LEGACY_CURRENCY_CONFIG })
    renderSwitch()
    expect(screen.queryByRole('group')).toBeNull()
  })
})
