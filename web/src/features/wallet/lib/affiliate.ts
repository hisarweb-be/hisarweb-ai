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
  convertHwUsd,
  niceFloor,
  type HwCurrency,
  type HwRates,
} from '@/features/hw/lib/currency'

// ============================================================================
// Affiliate Functions
// ============================================================================

/**
 * Generate affiliate registration link
 */
export function generateAffiliateLink(affCode: string): string {
  if (typeof window === 'undefined') return ''
  return `${window.location.origin}/sign-up?aff=${affCode}`
}

/** Share of the list value a friend's purchase pays out as credit (server: HwAffiliatePercent). */
export const AFFILIATE_PERCENT = 5

/** A friend's purchase worth this many USD is the example in the steps. */
const EXAMPLE_PURCHASE_USD = 20

/**
 * The example amounts of step three in the visitor's currency: a tidy purchase
 * and the credit it earns. Without a usable rate the example stays in USD.
 */
export function affiliateExample(
  currency: HwCurrency,
  rates: HwRates
): { currency: HwCurrency; paid: number; reward: number } {
  const converted = convertHwUsd(EXAMPLE_PURCHASE_USD, currency, rates)
  if (converted === null) {
    return {
      currency: 'USD',
      paid: EXAMPLE_PURCHASE_USD,
      reward: (EXAMPLE_PURCHASE_USD * AFFILIATE_PERCENT) / 100,
    }
  }
  const paid = niceFloor(converted)
  return { currency, paid, reward: (paid * AFFILIATE_PERCENT) / 100 }
}
