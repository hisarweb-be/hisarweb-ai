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
import type { HwTextKey } from '../texts'

/**
 * Formats a lira amount. The HisarWeb AI service is priced and charged in TRY,
 * whatever display currency the rest of the console is set to, so this does not
 * go through `@/lib/currency` (which follows that display setting).
 */
export function formatHwTry(amount: number, locale?: string): string {
  if (!Number.isFinite(amount)) return '-'
  const fractionDigits = Number.isInteger(amount) ? 0 : 2
  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency: 'TRY',
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  }).format(amount)
}

/** Formats a unix-seconds timestamp as a date; an empty stamp stays empty. */
export function formatHwDate(unixSeconds: number, locale?: string): string {
  if (!Number.isFinite(unixSeconds) || unixSeconds <= 0) return '-'
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(
    unixSeconds * 1000
  )
}

/** Text key of a package source; unknown sources are shown as the raw value. */
export function hwPackageSourceTextKey(source: string): HwTextKey | null {
  switch (source) {
    case 'purchase':
      return 'packageSourcePurchase'
    case 'free':
      return 'packageSourceFree'
    case 'affiliate':
      return 'packageSourceAffiliate'
    case 'admin':
      return 'packageSourceAdmin'
    default:
      return null
  }
}
