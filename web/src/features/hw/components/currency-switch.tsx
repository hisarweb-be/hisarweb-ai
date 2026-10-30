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
import { useEffect } from 'react'

import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'
import { useHwCurrencyStore } from '@/stores/hw-currency-store'

import { useHwCurrency } from '../hooks/use-hw-currency'
import {
  HW_CURRENCY_SYMBOLS,
  readHwProfileCurrency,
  type HwCurrency,
} from '../lib/currency'
import { useHwText, type HwTextKey } from '../texts'

const TITLE_KEYS: Record<HwCurrency, HwTextKey> = {
  TRY: 'currencyTitleTRY',
  EUR: 'currencyTitleEUR',
  USD: 'currencyTitleUSD',
}

// The profile choice is applied once per login: a later local change must not
// be undone by the header remounting with a not yet refreshed profile.
let profileAppliedFor: number | null = null

/**
 * Segmented currency switch (₺ / € / $) for the headers. Shows only the
 * currencies the server sells, and nothing when there is just one. The choice
 * is kept in this browser and, when signed in, in the profile.
 */
export function HwCurrencySwitch(props: { className?: string }) {
  const hw = useHwText()
  const { currency, currencies, setCurrency } = useHwCurrency()
  const user = useAuthStore((state) => state.auth.user)
  const userId = user?.id ?? null
  const profileCurrency = readHwProfileCurrency(user?.setting)

  useEffect(() => {
    if (userId === null) {
      profileAppliedFor = null
      return
    }
    if (profileAppliedFor === userId) return
    profileAppliedFor = userId
    if (profileCurrency) {
      useHwCurrencyStore.getState().setPreferred(profileCurrency)
    }
  }, [userId, profileCurrency])

  async function choose(next: HwCurrency) {
    setCurrency(next)
    if (userId === null) return
    try {
      await api.put('/api/user/self', { hw_currency: next })
    } catch {
      // Best-effort persistence; the local choice already applies.
    }
  }

  if (currencies.length < 2) return null

  return (
    <div
      role='group'
      aria-label={hw('currencySwitchLabel')}
      className={cn(
        'border-border/60 inline-flex items-center gap-0.5 rounded-lg border p-0.5',
        props.className
      )}
    >
      {currencies.map((code) => {
        const isActive = currency === code
        return (
          <Button
            key={code}
            type='button'
            size='sm'
            variant={isActive ? 'secondary' : 'ghost'}
            aria-pressed={isActive}
            title={hw(TITLE_KEYS[code])}
            onClick={() => choose(code)}
            className='h-7 w-6 rounded-md px-0 text-xs font-semibold sm:w-7'
          >
            {HW_CURRENCY_SYMBOLS[code]}
            <span className='sr-only'> {hw(TITLE_KEYS[code])}</span>
          </Button>
        )
      })}
    </div>
  )
}
