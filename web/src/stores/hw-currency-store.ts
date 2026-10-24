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
import { create } from 'zustand'

import {
  HW_CURRENCY_STORAGE_KEY,
  HW_LEGACY_CURRENCY_CONFIG,
  isHwCurrency,
  type HwCurrency,
  type HwCurrencyConfig,
} from '@/features/hw/lib/currency'

/**
 * The HisarWeb AI display currency: what the server sells (`config`, from
 * `/api/status`) and what the visitor picked (`preferred`). Plain store, no
 * react-query, so text hooks can read it in places that have no query client.
 */
type HwCurrencyState = {
  config: HwCurrencyConfig
  preferred: HwCurrency | null
  setConfig: (config: HwCurrencyConfig) => void
  setPreferred: (currency: HwCurrency) => void
}

function readStoredCurrency(): HwCurrency | null {
  try {
    if (typeof window === 'undefined') return null
    const stored = window.localStorage.getItem(HW_CURRENCY_STORAGE_KEY)
    return isHwCurrency(stored) ? stored : null
  } catch {
    return null
  }
}

export const useHwCurrencyStore = create<HwCurrencyState>((set) => ({
  config: HW_LEGACY_CURRENCY_CONFIG,
  preferred: readStoredCurrency(),
  setConfig: (config) => set({ config }),
  setPreferred: (currency) => {
    try {
      window.localStorage.setItem(HW_CURRENCY_STORAGE_KEY, currency)
    } catch {
      /* Storage can be unavailable in private mode; the choice still applies. */
    }
    set({ preferred: currency })
  },
}))
