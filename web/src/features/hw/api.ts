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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  HwApiEnvelope,
  HwCreditPackages,
  HwPriceComparison,
  HwQualityLatest,
  HwTopUpOrder,
  HwTopUpRequest,
} from './types'

/**
 * Bounds of the list value. They mirror the server options `HwMinTopUpTry` and
 * `HwMaxTopUpTry` (defaults 100 and 20000); the server still validates, this
 * only keeps the form from sending a value it must refuse.
 */
export const HW_TOPUP_MIN_TRY = 100
export const HW_TOPUP_MAX_TRY = 20000

/** Preset list values in whole lira, all inside the bounds above. */
export const HW_TOPUP_PRESET_TRY = [250, 500, 1000, 2000, 5000]

export async function fetchHwCreditPackages(): Promise<HwCreditPackages> {
  const response = await api.get('/api/user/hw/packages')
  return requireServerSuccess(response.data).data as HwCreditPackages
}

export async function createHwTopUpOrder(
  request: HwTopUpRequest
): Promise<HwTopUpOrder> {
  const response = await api.post('/api/user/hw/orders', request)
  return requireServerSuccess(response.data).data as HwTopUpOrder
}

export type HwCompareRequest = {
  model: string
  input_tokens: number
  output_tokens: number
}

/** Public price comparison; unknown or unsold models answer 404. */
export async function compareHwPrices(
  request: HwCompareRequest
): Promise<HwApiEnvelope<HwPriceComparison>> {
  const res = await api.get('/api/hw/pricing/compare', { params: request })
  return requireServerSuccess(res.data as HwApiEnvelope<HwPriceComparison>)
}

/** Public quality summary of the newest measurement. */
export async function getHwQualityLatest(): Promise<
  HwApiEnvelope<HwQualityLatest>
> {
  const res = await api.get('/api/hw/quality/latest')
  return requireServerSuccess(res.data as HwApiEnvelope<HwQualityLatest>)
}
