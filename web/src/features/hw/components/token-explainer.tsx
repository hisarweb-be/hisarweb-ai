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
import { Calculator } from 'lucide-react'
import { useId, useState } from 'react'

import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { TitledCard } from '@/components/ui/titled-card'
import { cn } from '@/lib/utils'

import { MODEL_PRICES } from '../data/model-prices'
import { useHwCurrency } from '../hooks/use-hw-currency'
import { formatHwAmount } from '../lib/format'
import {
  ACTIVE_HOURS_PER_DAY,
  HEAVY_USD_PER_ACTIVE_DAY,
  HOURS_PER_SMALL_PROJECT,
  REFERENCE_MODEL,
  TYPICAL_USD_PER_ACTIVE_DAY,
  USAGE_SOURCE_URL,
  estimateUsage,
  formatEstimate,
  usageBudgetsIn,
  usageRatePerUsd,
} from '../lib/usage-calculator'
import { useHwText } from '../texts'

type TokenExplainerProps = {
  /** Intl locale used for the numbers. */
  locale?: string
  className?: string
}

const NOTES = [
  ['explainTokenTitle', 'explainTokenBody'],
  ['explainIoTitle', 'explainIoBody'],
  ['explainCodingTitle', 'explainCodingBody'],
] as const

/**
 * Explains tokens in plain words and lets a beginner see how long a budget
 * lasts on each model. Prices come from the model price table; the usage
 * assumption is shown next to every result, with its source.
 */
export function TokenExplainer(props: TokenExplainerProps) {
  const hw = useHwText()
  const id = useId()
  const { currency, rates } = useHwCurrency()
  const budgets = usageBudgetsIn(currency, rates)
  // The selected button is kept by position, so a currency switch keeps it.
  const [budgetIndex, setBudgetIndex] = useState(1)
  const budget = budgets[budgetIndex] ?? budgets[0]
  const [model, setModel] = useState<string>(REFERENCE_MODEL)

  const estimate = estimateUsage(
    budget,
    model,
    usageRatePerUsd(currency, rates)
  )

  return (
    <TitledCard
      title={hw('explainTitle')}
      description={hw('explainLead')}
      icon={<Calculator className='h-4 w-4' />}
      iconTone='info'
      disableHoverEffect
      className={cn('w-full', props.className)}
    >
      <div className='flex flex-col gap-5'>
        <dl className='grid gap-3 sm:grid-cols-3'>
          {NOTES.map(([titleKey, bodyKey]) => (
            <div key={titleKey} className='bg-muted/40 rounded-lg p-3'>
              <dt className='text-sm font-medium'>{hw(titleKey)}</dt>
              <dd className='text-muted-foreground mt-1 text-sm'>
                {hw(bodyKey)}
              </dd>
            </div>
          ))}
        </dl>

        <section aria-labelledby={`${id}-calc`} className='flex flex-col gap-3'>
          <h2 id={`${id}-calc`} className='text-sm font-semibold'>
            {hw('calcTitle')}
          </h2>
          <div className='flex flex-wrap items-end gap-4'>
            <div className='flex flex-col gap-1.5'>
              <span className='text-sm font-medium'>
                {hw('calcBudgetLabel')}
              </span>
              <div
                role='group'
                aria-label={hw('calcBudgetLabel')}
                className='flex flex-wrap gap-2'
              >
                {budgets.map((value, index) => (
                  <Button
                    key={value}
                    type='button'
                    size='sm'
                    variant={budgetIndex === index ? 'default' : 'outline'}
                    aria-pressed={budgetIndex === index}
                    onClick={() => setBudgetIndex(index)}
                  >
                    {formatHwAmount(value, currency, props.locale)}
                  </Button>
                ))}
              </div>
            </div>
            <div className='flex flex-col gap-1.5'>
              <Label htmlFor={`${id}-model`}>{hw('calcModelLabel')}</Label>
              <select
                id={`${id}-model`}
                value={model}
                onChange={(event) => setModel(event.target.value)}
                className='border-input bg-background h-8 rounded-md border px-2 text-sm'
              >
                {MODEL_PRICES.map((price) => (
                  <option key={price.model} value={price.model}>
                    {price.model}
                  </option>
                ))}
              </select>
            </div>
          </div>

          {estimate ? (
            <div aria-live='polite' className='flex flex-col gap-1'>
              <p className='text-base font-semibold'>
                {hw('calcHoursResult', {
                  low: formatEstimate(estimate.hoursLow, props.locale),
                  high: formatEstimate(estimate.hoursHigh, props.locale),
                })}
              </p>
              <p className='text-base font-semibold'>
                {hw('calcProjectsResult', {
                  low: formatEstimate(estimate.projectsLow, props.locale),
                  high: formatEstimate(estimate.projectsHigh, props.locale),
                })}
              </p>
            </div>
          ) : (
            <p className='text-muted-foreground text-sm'>
              {hw('calcUnavailable')}
            </p>
          )}

          <p className='text-muted-foreground text-xs'>
            {hw('calcAssumption', {
              typical: TYPICAL_USD_PER_ACTIVE_DAY,
              heavy: HEAVY_USD_PER_ACTIVE_DAY,
              hours: ACTIVE_HOURS_PER_DAY,
              reference: REFERENCE_MODEL,
              projectHours: HOURS_PER_SMALL_PROJECT,
            })}{' '}
            <a
              href={USAGE_SOURCE_URL}
              target='_blank'
              rel='noreferrer'
              className='underline'
            >
              {hw('calcSourceLabel')}
            </a>
          </p>
        </section>
      </div>
    </TitledCard>
  )
}
