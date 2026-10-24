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
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { toIntlLocale } from '@/i18n/languages'
import { cn } from '@/lib/utils'

import { formatHwAmount } from '../lib/format'
import { useHwBonusRules } from '../lib/hw-bonus'
import { useHwText } from '../texts'

interface BonusExplainerProps {
  className?: string
}

/**
 * Explains the top-up bonus and how long a balance stays valid. Every amount
 * and the number of days come from `useHwBonusRules()`; the texts only hold
 * placeholders. Without bonus rules (the bonus is switched off) only the
 * validity note remains.
 */
export function BonusExplainer(props: BonusExplainerProps) {
  const hw = useHwText()
  const { i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage ?? i18n.language)
  const { rules } = useHwBonusRules()
  const titleId = useId()
  const hasRules = rules.length > 0

  return (
    <section
      aria-labelledby={hasRules ? titleId : undefined}
      className={cn(
        'bg-muted/40 flex flex-col gap-2 rounded-md p-3 text-sm',
        props.className
      )}
    >
      {hasRules && (
        <>
          <h3 id={titleId} className='flex items-center gap-2 font-medium'>
            <Gift className='size-4 shrink-0' aria-hidden='true' />
            {hw('bonusExplainTitle')}
          </h3>
          <p>{hw('bonusExplainLead')}</p>
          <ul className='list-disc space-y-0.5 pl-5'>
            {rules.map((rule) => (
              <li key={rule.currency}>
                {hw('bonusRuleLine', {
                  min: formatHwAmount(rule.min, rule.currency, locale),
                  amount: formatHwAmount(rule.amount, rule.currency, locale),
                })}
              </li>
            ))}
          </ul>
        </>
      )}
      <p className='text-muted-foreground text-xs'>{hw('bonusValidityNote')}</p>
    </section>
  )
}
