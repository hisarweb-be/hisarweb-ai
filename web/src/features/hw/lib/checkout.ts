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
import { getServerErrorStatus } from '@/lib/server-error-message'

import type { HwTextKey } from '../texts'

/**
 * Sends the browser to the checkout page the order answer points at. The
 * provider hosts that page, so it leaves the router.
 */
export function openCheckout(url: string): void {
  window.location.assign(url)
}

/**
 * True when the page was opened from the payment provider's return URL.
 *
 * The return URL is the server option `HwCheckoutReturnURL` and the provider
 * appends its own query parameters to it (the order reference); this page never
 * adds any itself. So any query parameter means the customer came back from the
 * payment page — enough to word the notice, never enough to claim a payment
 * succeeded.
 */
export function hasPaymentReturnQuery(search: string): boolean {
  return [...new URLSearchParams(search).keys()].length > 0
}

/**
 * Maps a failed top-up order to the sentence the customer reads. The server
 * answers 400 (value outside the bounds), 409 (an order is already waiting for
 * payment) and 503 (no rate or no payment provider); everything else is
 * reported as a plain failure instead of quoting the server's English text.
 */
export function hwOrderErrorTextKey(error: unknown): HwTextKey {
  const status = getServerErrorStatus(error)
  if (status === 400) return 'topUpErrorRange'
  if (status === 409) return 'topUpErrorOpenOrders'
  if (status === 503) return 'topUpErrorUnavailable'
  return 'topUpErrorDefault'
}
