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
import { Gift } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import { formatHwAmount } from '../lib/format'
import type { HwCurrency } from '../lib/currency'

interface BonusBadgeProps {
  currency: HwCurrency
  /** Bonus in whole units of `currency`. Nothing is drawn for zero. */
  amount: number
  locale?: string
  className?: string
}

/** "+₺50 bonus": what a top-up earns on top of the amount paid. */
export function HwBonusBadge(props: BonusBadgeProps) {
  const { t } = useTranslation()
  if (!(props.amount > 0)) return null
  const text = t('+{{amount}} bonus', {
    amount: formatHwAmount(props.amount, props.currency, props.locale),
  })
  return (
    <Badge variant='warning' className={props.className} aria-label={text}>
      <Gift aria-hidden='true' />
      {text}
    </Badge>
  )
}

/** The badge of a finished order, whose bonus is stored in minor units. */
export function HwOrderBonusBadge(props: {
  currency: HwCurrency
  bonusMinor: number
  locale?: string
}) {
  return (
    <HwBonusBadge
      currency={props.currency}
      amount={props.bonusMinor / 100}
      locale={props.locale}
    />
  )
}

