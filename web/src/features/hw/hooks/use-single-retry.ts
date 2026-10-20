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
import { useEffect, useRef } from 'react'

/**
 * One automatic retry after a short pause for a failed read.
 *
 * A public page that keeps saying "yükleniyor…" after one failed request
 * leaves the visitor with nothing to do; a rate limit or a restarting service
 * often clears within seconds. This gives the query exactly one extra attempt
 * per failure and leaves further attempts to the visitor's own button.
 */
export function useSingleRetry(
  retry: () => void,
  isError: boolean,
  delayMs: number
) {
  const alreadyRetried = useRef(false)

  useEffect(() => {
    if (!isError) {
      alreadyRetried.current = false
      return
    }
    if (alreadyRetried.current) {
      return
    }
    const timer = window.setTimeout(() => {
      alreadyRetried.current = true
      retry()
    }, delayMs)
    return () => window.clearTimeout(timer)
  }, [delayMs, isError, retry])
}
