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
/** Shapes of the HisarWeb AI top-up routes (`router/hw-revolut.go`). */

import type { HwLanguage } from './texts'

/** One credit package as `GET /api/user/hw/packages` reports it. */
export interface HwCreditPackage {
  /** `purchase`, `free`, `affiliate` or `admin`. */
  source: string
  granted_quota: number
  remaining_quota: number
  /** Unix seconds. */
  created_at: number
  /** Unix seconds; the balance is spent before this moment. */
  expires_at: number
}

/** Answer of `GET /api/user/hw/packages`. */
export interface HwCreditPackages {
  packages: HwCreditPackage[]
  total_remaining_quota: number
  /** Quota units per US dollar. */
  quota_per_unit: number
  /** TRY per USD from the last price snapshot; empty when none exists yet. */
  try_per_usd: string
  /** Date of the rate (`YYYY-MM-DD`); empty when no snapshot exists yet. */
  fx_date: string
  fx_source: string
}

/**
 * Body of `POST /api/user/hw/orders`. `package_try` is the list value in whole
 * lira; the server computes the charged amount and the credit.
 */
export interface HwTopUpRequest {
  package_try: number
  language: HwLanguage
}

/** Answer of `POST /api/user/hw/orders`. */
export interface HwTopUpOrder {
  merchant_ref: string
  checkout_url: string
}
