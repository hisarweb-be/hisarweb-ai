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
import { CalendarClock } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { HW_VALIDITY_WARNING_DAYS } from '../lib/bonus'
import { formatHwDate } from '../lib/format'
import type { HwCreditValidity } from '../types'

interface CreditValidityCardProps {
  validity?: HwCreditValidity
  locale?: string
  className?: string
}

/**
 * "Kredi geçerliliği: X gün kaldı": the countdown to the day the balance
 * lapses. Every paid top-up moves that day to 30 days ahead, which the text
 * says so the customer knows how to keep the balance. Red at three days or
 * less and once the balance has lapsed. Draws nothing without a balance and
 * without a lapsed one, and nothing on an older server that sends no validity.
 */
export function CreditValidityCard(props: CreditValidityCardProps) {
  const { t } = useTranslation()
  const validity = props.validity
  if (!validity || validity.status === 'none') return null

  const expired = validity.status === 'expired'
  const urgent = expired || validity.days_remaining <= HW_VALIDITY_WARNING_DAYS
  let state = 'ok'
  if (expired) state = 'expired'
  else if (urgent) state = 'urgent'

  return (
    <section
      aria-label={t('Credit validity')}
      className={cn(
        'flex items-start gap-3 rounded-xl border p-4',
        urgent
          ? 'border-destructive/40 bg-destructive/10 text-destructive'
          : 'bg-card',
        props.className
      )}
      data-state={state}
    >
      <CalendarClock className='mt-0.5 size-5 shrink-0' aria-hidden='true' />
      <div className='flex flex-col gap-1'>
        <p className='text-base font-semibold'>
          {expired
            ? t('Credit validity: expired')
            : t('Credit validity: {{days}} days left', {
                days: validity.days_remaining,
              })}
        </p>
        <p
          className={cn(
            'text-sm',
            urgent ? 'text-destructive/90' : 'text-muted-foreground'
          )}
        >
          {expired
            ? t(
                'Your credit expired on {{date}} and was removed from the balance. A new top-up gives you a new 30 days.',
                { date: formatHwDate(validity.expired_at, props.locale) }
              )
            : t(
                'Your credit is valid until {{date}}. Every top-up renews all of it for 30 days.',
                { date: formatHwDate(validity.valid_until, props.locale) }
              )}
        </p>
      </div>
    </section>
  )
}
