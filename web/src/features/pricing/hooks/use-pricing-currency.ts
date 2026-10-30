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

import { useHwCurrency } from '@/features/hw/hooks/use-hw-currency'
import { useSystemConfigStore } from '@/stores/system-config-store'

import {
  resolvePricingCurrencyOverride,
  type PricingCurrencyOverride,
} from '../lib/currency'

/**
 * The currency inputs of a price: the console display setting and the header
 * switch. Subscribing here re-renders a price when either changes; the object
 * only changes identity then, so it also works as a `useMemo` dependency of the
 * formatters in `lib/price.ts`, which read the same stores.
 */
export function usePricingCurrency() {
  const system = useSystemConfigStore((state) => state.config.currency)
  const { currency, rates } = useHwCurrency()
  return useMemo<{
    system: typeof system
    override: PricingCurrencyOverride | null
  }>(
    () => ({
      system,
      override: resolvePricingCurrencyOverride(currency, rates),
    }),
    [system, currency, rates]
  )
}
