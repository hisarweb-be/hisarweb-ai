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
import { RefreshCw, Wallet } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'
import { formatNumber } from '@/lib/format'

import { formatHwDate, formatHwTry, hwPackageSourceTextKey } from '../lib/format'
import { useHwText } from '../texts'
import type { HwCreditPackages } from '../types'

interface CreditPackagesCardProps {
  data?: HwCreditPackages
  isLoading: boolean
  isError: boolean
  onRefresh: () => void
  locale?: string
}

/**
 * Shows the balance per package with its expiry date. The lira value follows
 * from the rate of the last price snapshot; without one the page says so
 * instead of inventing an amount.
 */
export function CreditPackagesCard(props: CreditPackagesCardProps) {
  const hw = useHwText()
  const rate = Number(props.data?.try_per_usd ?? '')
  const quotaPerUnit = props.data?.quota_per_unit ?? 0
  const rateIsUsable =
    Number.isFinite(rate) && rate > 0 && quotaPerUnit > 0

  function liraOf(quota: number): string | null {
    if (!rateIsUsable) return null
    return formatHwTry((quota / quotaPerUnit) * rate, props.locale)
  }

  function amountOf(quota: number): string {
    return (
      liraOf(quota) ??
      hw('balancePackageQuota', { quota: formatNumber(quota, props.locale) })
    )
  }

  return (
    <TitledCard
      title={hw('topUpPackagesTitle')}
      description={hw('topUpPackagesLead')}
      icon={<Wallet className='h-4 w-4' />}
      iconTone='success'
      disableHoverEffect
      action={
        <Button variant='outline' size='sm' onClick={props.onRefresh}>
          <RefreshCw className='size-4' aria-hidden='true' />
          {hw('topUpRefresh')}
        </Button>
      }
    >
      {props.isLoading && (
        <div className='flex flex-col gap-2' aria-busy='true'>
          <Skeleton className='h-4 w-2/3' />
          <Skeleton className='h-4 w-1/2' />
          <span className='sr-only'>{hw('topUpPackagesLoading')}</span>
        </div>
      )}

      {!props.isLoading && props.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {hw('topUpPackagesError')}
        </p>
      )}

      {!props.isLoading && !props.isError && (
        <PackageList
          data={props.data}
          amountOf={amountOf}
          hasRate={rateIsUsable}
          locale={props.locale}
        />
      )}
    </TitledCard>
  )
}

interface PackageListProps {
  data?: HwCreditPackages
  amountOf: (quota: number) => string
  hasRate: boolean
  locale?: string
}

function PackageList(props: PackageListProps) {
  const hw = useHwText()
  const packages = props.data?.packages ?? []

  if (packages.length === 0) {
    return <p className='text-muted-foreground text-sm'>{hw('balanceEmpty')}</p>
  }

  return (
    <div className='flex flex-col gap-3'>
      <p className='text-sm font-medium'>
        {hw('balanceTotal', {
          amount: props.amountOf(props.data?.total_remaining_quota ?? 0),
        })}
      </p>
      <ul className='flex flex-col gap-2'>
        {packages.map((creditPackage) => {
          const sourceKey = hwPackageSourceTextKey(creditPackage.source)
          return (
            <li
              key={`${creditPackage.source}-${creditPackage.expires_at}-${creditPackage.granted_quota}`}
              className='bg-muted/40 rounded-md px-3 py-2 text-sm'
            >
              {hw('balancePackageLine', {
                source: sourceKey ? hw(sourceKey) : creditPackage.source,
                amount: props.amountOf(creditPackage.remaining_quota),
                date: formatHwDate(creditPackage.expires_at, props.locale),
              })}
            </li>
          )
        })}
      </ul>
      {!props.hasRate && (
        <p className='text-muted-foreground text-xs'>{hw('balanceNoRate')}</p>
      )}
    </div>
  )
}
