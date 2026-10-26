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
import { Receipt } from 'lucide-react'

import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'

import { formatHwAmount, formatHwDate } from '../lib/format'
import { useHwText } from '../texts'
import type { HwOrderSummary } from '../types'
import { HwOrderBonusBadge } from './bonus-badge'

interface OrderHistoryCardProps {
  orders?: HwOrderSummary[]
  isLoading: boolean
  isError: boolean
  locale?: string
}

/** The customer's paid and refunded orders, each with the bonus it earned. */
export function OrderHistoryCard(props: OrderHistoryCardProps) {
  const hw = useHwText()
  const orders = props.orders ?? []

  return (
    <TitledCard
      title={hw('orderHistoryTitle')}
      description={hw('orderHistoryLead')}
      icon={<Receipt className='h-4 w-4' />}
      iconTone='success'
      disableHoverEffect
    >
      {props.isLoading && (
        <div className='flex flex-col gap-2' aria-busy='true'>
          <Skeleton className='h-4 w-2/3' />
          <Skeleton className='h-4 w-1/2' />
          <span className='sr-only'>{hw('orderHistoryLoading')}</span>
        </div>
      )}

      {!props.isLoading && props.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {hw('orderHistoryError')}
        </p>
      )}

      {!props.isLoading && !props.isError && orders.length === 0 && (
        <p className='text-muted-foreground text-sm'>
          {hw('orderHistoryEmpty')}
        </p>
      )}

      {!props.isLoading && !props.isError && orders.length > 0 && (
        <ul className='flex flex-col gap-2'>
          {orders.map((order) => (
            <li
              key={order.merchant_ref}
              className='bg-muted/40 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md px-3 py-2 text-sm'
            >
              <span>
                {hw('orderHistoryLine', {
                  date: formatHwDate(order.paid_at, props.locale),
                  amount: formatHwAmount(
                    order.payable_minor / 100,
                    order.currency,
                    props.locale
                  ),
                  status:
                    order.status === 'refunded'
                      ? hw('orderStatusRefunded')
                      : hw('orderStatusPaid'),
                })}
              </span>
              <HwOrderBonusBadge
                currency={order.currency}
                bonusMinor={order.bonus_minor}
                locale={props.locale}
              />
            </li>
          ))}
        </ul>
      )}
    </TitledCard>
  )
}
