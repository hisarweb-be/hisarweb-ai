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
import {
  HW_CURRENCY_SYMBOLS,
  convertHwUsd,
  resolveHwCurrency,
  type HwCurrency,
  type HwRates,
} from '@/features/hw/lib/currency'
import {
  formatBillingCurrencyFromUSD,
  formatCurrencyAmount,
  type CurrencyFormatOptions,
} from '@/lib/currency'
import { useHwCurrencyStore } from '@/stores/hw-currency-store'

/**
 * Model prices follow the currency switch in the header.
 *
 * The console's display setting (`quota_display_type`, lira on the live site)
 * only describes one currency. When the visitor picks EUR or USD, prices are
 * converted with the server's rate for that currency, exactly like the price
 * calculator. TRY, and an older server that sends no currency data, keep
 * using the console setting, so nothing changes there.
 */
export type PricingCurrencyOverride = {
  code: Exclude<HwCurrency, 'TRY'>
  symbol: string
  /** Units of `code` per 1 USD. */
  rate: number
}

/** The EUR/USD override of the chosen currency, or null to use the console setting. */
export function resolvePricingCurrencyOverride(
  currency: HwCurrency,
  rates: HwRates
): PricingCurrencyOverride | null {
  if (currency === 'TRY') return null
  const rate = convertHwUsd(1, currency, rates)
  if (rate === null) return null
  return { code: currency, symbol: HW_CURRENCY_SYMBOLS[currency], rate }
}

/** Same as `resolvePricingCurrencyOverride`, read from the live store. */
function getPricingCurrencyOverride(): PricingCurrencyOverride | null {
  const { config, preferred } = useHwCurrencyStore.getState()
  return resolvePricingCurrencyOverride(
    resolveHwCurrency(config, preferred),
    config.rates
  )
}

/**
 * `formatBillingCurrencyFromUSD` for model prices: shows the currency chosen
 * with the header switch (EUR/USD), otherwise the console display currency.
 */
export function formatPricingCurrencyFromUSD(
  amountUSD: number | null | undefined,
  options?: CurrencyFormatOptions
): string {
  const override = getPricingCurrencyOverride()
  if (!override) return formatBillingCurrencyFromUSD(amountUSD, options)
  if (amountUSD == null || Number.isNaN(amountUSD)) return '-'
  return formatCurrencyAmount(amountUSD * override.rate, override.code, options)
}
