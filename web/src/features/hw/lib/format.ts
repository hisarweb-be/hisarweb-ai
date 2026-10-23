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
import type { HwLanguage, HwTextKey, HwTextVars } from '../texts'
import { formatDateTimeStr } from '@/lib/format'
import { HW_COMPARE_MAX_MONTHLY_TOKENS } from '../constants'
import type { HwMoneyText } from '../types'
import type { HwCurrency } from './currency'

/**
 * Formats an amount that is already in `currency` (TRY, EUR or USD): whole
 * amounts without decimals, anything else with two. The HisarWeb AI service has
 * its own currency, so this does not go through `@/lib/currency` (which follows
 * the console's display setting). `locale` must already be an Intl locale.
 */
export function formatHwAmount(
  amount: number,
  currency: HwCurrency,
  locale?: string
): string {
  if (!Number.isFinite(amount)) return '-'
  const fractionDigits = Number.isInteger(amount) ? 0 : 2
  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency,
    currencyDisplay: currency === 'TRY' ? 'symbol' : 'narrowSymbol',
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

/**
 * Reads one of the API's decimal money strings. The API never sends money as a
 * JSON number, so a missing or malformed value stays `null` and the page shows
 * a dash instead of an invented amount.
 */
export function parseHwAmount(
  value: HwMoneyText | null | undefined
): number | null {
  if (value == null || value.trim() === '') return null
  const amount = Number(value)
  return Number.isFinite(amount) ? amount : null
}

/**
 * Formats an API amount that is already in `currency` (the official USD price,
 * the TRY prices, or a USD price converted with `hw_rates`). The shared currency
 * helpers convert quota into the admin's display currency, which is not what
 * these amounts are, so this page formats the amount directly. Sub-unit amounts
 * keep six decimals so a cheap request never displays as zero.
 */
export function formatHwMoney(
  value: number | null,
  currency: HwCurrency,
  locale?: string
): string {
  if (value == null || !Number.isFinite(value)) return '-'
  const maximumFractionDigits =
    Math.abs(value) > 0 && Math.abs(value) < 1 ? 6 : 2
  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency,
    currencyDisplay: currency === 'TRY' ? 'symbol' : 'narrowSymbol',
    minimumFractionDigits: 2,
    maximumFractionDigits,
  }).format(value)
}

/** Reads a monthly token count typed by a visitor: whole, within the bound. */
export function parseHwTokenCount(raw: string): number | null {
  const trimmed = raw.trim()
  if (!/^\d+$/.test(trimmed)) return null
  const value = Number(trimmed)
  if (!Number.isSafeInteger(value) || value > HW_COMPARE_MAX_MONTHLY_TOKENS) {
    return null
  }
  return value
}

/**
 * Formats a date the API sends either as Unix seconds (`measured_at`), as an
 * RFC 3339 timestamp (`price_fetched_at`) or as a plain `YYYY-MM-DD` day
 * (`fx_date`). A bare day is shown as written so a timezone shift cannot move
 * the exchange-rate date to another day.
 */
export function formatHwStamp(
  value: string | number | null | undefined
): string {
  if (value == null || value === '') return '-'
  if (typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value.trim())) {
    return value.trim()
  }
  const date =
    typeof value === 'number' ? new Date(value * 1000) : new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return formatDateTimeStr(date)
}

/** Text key of a model state in the public status answer. */
export function hwStatusTextKey(state: string): HwTextKey {
  switch (state) {
    case 'degraded':
      return 'statusDegraded'
    case 'outage':
      return 'statusOutage'
    default:
      return 'statusOperational'
  }
}

/**
 * Reads an outage length in seconds as minutes and hours. The server rounds a
 * length up to whole minutes, so this does the same: an outage of 61 seconds
 * reads as 2 minutes and not as "1 minute".
 */
export function formatHwDuration(
  seconds: number,
  text: (key: HwTextKey, vars?: HwTextVars) => string
): string {
  if (!Number.isFinite(seconds) || seconds < 60) {
    return text('statusDurationBelowMinute')
  }
  const minutes = Math.ceil(seconds / 60)
  if (minutes < 60) return text('statusDurationMinutes', { minutes })
  const hours = Math.floor(minutes / 60)
  const rest = minutes % 60
  if (rest === 0) return text('statusDurationHoursOnly', { hours })
  return text('statusDurationHours', { hours, minutes: rest })
}

/**
 * The message the server wrote for the page language. Turkish is the default of
 * the site, so a missing Turkish text falls back to the English one.
 */
export function hwStatusMessage(
  entry: { message_tr: string; message_en: string },
  language: HwLanguage
): string {
  if (language === 'en') return entry.message_en || entry.message_tr
  return entry.message_tr || entry.message_en
}
