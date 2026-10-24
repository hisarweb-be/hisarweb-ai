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
import type { HwBonusRule } from '../types'
import type { HwCurrency } from './currency'

/** The bonus a payment of `amount` (whole units of `currency`) earns, or 0. */
export function hwBonusFor(
  rules: readonly HwBonusRule[] | undefined,
  currency: HwCurrency,
  amount: number
): number {
  const rule = rules?.find((entry) => entry.currency === currency)
  if (!rule || rule.min <= 0 || rule.amount <= 0) return 0
  return amount >= rule.min ? rule.amount : 0
}

/** From this many days left the validity card turns red. */
export const HW_VALIDITY_WARNING_DAYS = 3
