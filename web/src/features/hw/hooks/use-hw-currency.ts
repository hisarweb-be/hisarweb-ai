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
import { useMemo } from 'react'

import { useHwCurrencyStore } from '@/stores/hw-currency-store'

import {
  HW_LEGACY_TRY_TOPUP,
  resolveHwCurrency,
  type HwCurrency,
  type HwRates,
  type HwTopUpLimits,
} from '../lib/currency'

export type HwCurrencyState = {
  /** The active currency: the visitor's pick, or the detected default. */
  currency: HwCurrency
  /** Currencies on sale, in switch order. */
  currencies: HwCurrency[]
  /** Units of currency per 1 USD (USD is 1). */
  rates: HwRates
  /** Top-up limits of the active currency. */
  topUp: HwTopUpLimits
  setCurrency: (currency: HwCurrency) => void
}

/** The one place every HisarWeb component reads its currency from. */
export function useHwCurrency(): HwCurrencyState {
  const config = useHwCurrencyStore((state) => state.config)
  const preferred = useHwCurrencyStore((state) => state.preferred)
  const setCurrency = useHwCurrencyStore((state) => state.setPreferred)
  const currency = useMemo(
    () => resolveHwCurrency(config, preferred),
    [config, preferred]
  )
  return {
    currency,
    currencies: config.currencies,
    rates: config.rates,
    topUp: config.topUp[currency] ?? HW_LEGACY_TRY_TOPUP,
    setCurrency,
  }
}
