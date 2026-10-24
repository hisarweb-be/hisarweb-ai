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

import { useHwBonusStore } from '@/stores/hw-bonus-store'

import { isHwCurrency, type HwCurrency } from './currency'

/**
 * Top-up bonus rules and the credit validity of the HisarWeb AI pages. They
 * come from the server; no page or text writes the numbers itself.
 *
 * Sources, strongest first:
 * 1. `bonus_rules` of `GET /api/user/hw/packages` (signed-in pages).
 * 2. `hw_bonus_rules` and `hw_credit_validity_days` of the public
 *    `GET /api/status` (landing page, not signed in).
 * 3. The fallback constants below, for fields the server does not send yet.
 */

/** One bonus rule: a top-up of `min` or more (whole units) earns `amount`. */
export type HwBonusRule = {
  currency: HwCurrency
  min: number
  amount: number
}

/** What the API delivered; `null` means the field is not in the answer. */
export type HwBonusApiConfig = {
  rules: HwBonusRule[] | null
  validityDays: number | null
}

const EMPTY_HW_BONUS_API: HwBonusApiConfig = { rules: null, validityDays: null }

// tijdelijk, vervangen zodra de API de velden levert
export const FALLBACK_BONUS_RULES: readonly HwBonusRule[] = [
  { currency: 'USD', min: 10, amount: 1 },
  { currency: 'EUR', min: 10, amount: 1 },
  { currency: 'TRY', min: 500, amount: 50 },
]

// tijdelijk, vervangen zodra de API de velden levert
export const FALLBACK_VALIDITY_DAYS = 30

function positiveNumber(value: unknown): number | null {
  const parsed = typeof value === 'string' ? Number(value) : value
  if (typeof parsed !== 'number' || !Number.isFinite(parsed) || parsed <= 0) {
    return null
  }
  return parsed
}

function parseRule(value: unknown): HwBonusRule | null {
  if (typeof value !== 'object' || value === null) return null
  const raw = value as Record<string, unknown>
  const min = positiveNumber(raw.min)
  const amount = positiveNumber(raw.amount)
  if (!isHwCurrency(raw.currency) || min === null || amount === null) {
    return null
  }
  return { currency: raw.currency, min, amount }
}

/**
 * Reads a rule list. Anything that is not a list stays `null` (the next source
 * applies); a list stays a list even when it is empty, because a rule with
 * amount 0 means the bonus is switched off for that currency.
 */
export function parseHwBonusRules(value: unknown): HwBonusRule[] | null {
  if (!Array.isArray(value)) return null
  return value
    .map(parseRule)
    .filter((rule): rule is HwBonusRule => rule !== null)
}

/** Reads the two public fields out of the `/api/status` answer. */
export function parseHwBonusConfig(status: unknown): HwBonusApiConfig {
  if (typeof status !== 'object' || status === null) return EMPTY_HW_BONUS_API
  const raw = status as Record<string, unknown>
  const days = positiveNumber(raw.hw_credit_validity_days)
  return {
    rules: parseHwBonusRules(raw.hw_bonus_rules),
    validityDays: days !== null && Number.isInteger(days) ? days : null,
  }
}

export type HwBonusRules = {
  rules: readonly HwBonusRule[]
  validityDays: number
}

/** The API values win; the fallback fills only what the API did not send. */
export function resolveHwBonus(
  status: HwBonusApiConfig,
  packageRules: HwBonusRule[] | null = null
): HwBonusRules {
  return {
    rules: packageRules ?? status.rules ?? FALLBACK_BONUS_RULES,
    validityDays: status.validityDays ?? FALLBACK_VALIDITY_DAYS,
  }
}

/**
 * The bonus rules and the credit validity to show. Reads the store that the
 * shared `/api/status` request and the packages request fill, so it needs no
 * query client and works in the text hooks as well.
 */
export function useHwBonusRules(): HwBonusRules {
  const status = useHwBonusStore((state) => state.status)
  const packageRules = useHwBonusStore((state) => state.packageRules)
  return useMemo(
    () => resolveHwBonus(status, packageRules),
    [status, packageRules]
  )
}
