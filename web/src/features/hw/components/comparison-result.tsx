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
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { formatNumber } from '@/lib/format'

import { formatHwStamp, formatHwMoney, parseHwAmount } from '../lib/format'
import { useHwText } from '../texts'
import type { HwPriceComparison } from '../types'

type ComparisonResultProps = {
  comparison: HwPriceComparison
  locale?: string
}

function ComparisonRow(props: { label: string; value: React.ReactNode }) {
  return (
    <div className='flex flex-col gap-0.5 sm:flex-row sm:justify-between sm:gap-4'>
      <dt className='text-muted-foreground'>{props.label}</dt>
      <dd className='font-medium break-words sm:text-right'>{props.value}</dd>
    </div>
  )
}

/** A source is a URL in practice; anything else is shown as plain text. */
function SourceValue(props: { value: string }) {
  const value = props.value.trim()
  if (value === '') return <span>-</span>
  if (!/^https?:\/\//i.test(value)) return <span>{value}</span>
  return (
    <a
      className='text-primary underline underline-offset-2'
      href={value}
      rel='noreferrer noopener'
      target='_blank'
    >
      {value}
    </a>
  )
}

export function ComparisonResult(props: ComparisonResultProps) {
  const t = useHwText()
  const comparison = props.comparison
  const money = (value: string, currency: 'TRY' | 'USD') =>
    formatHwMoney(parseHwAmount(value), currency, props.locale)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('compareResultTitle')}</CardTitle>
        <CardDescription>
          {t('compareResultLead', {
            model: comparison.model,
            input: formatNumber(comparison.input_tokens, props.locale),
            output: formatNumber(comparison.output_tokens, props.locale),
          })}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-4'>
        {comparison.stale && (
          <Alert variant='destructive'>
            <AlertTitle>{t('compareStaleTitle')}</AlertTitle>
            <AlertDescription>{t('compareStale')}</AlertDescription>
          </Alert>
        )}

        <dl className='space-y-2'>
          <ComparisonRow
            label={t('compareOfficialUsd')}
            value={money(comparison.official_cost_usd, 'USD')}
          />
          <ComparisonRow
            label={t('compareOfficialTry')}
            value={money(comparison.official_cost_try, 'TRY')}
          />
          <ComparisonRow
            label={t('compareOursFirst')}
            value={money(comparison.first_purchase_try, 'TRY')}
          />
          <ComparisonRow
            label={t('compareOursRepeat')}
            value={money(comparison.repeat_purchase_try, 'TRY')}
          />
        </dl>

        <div className='space-y-1 font-medium'>
          <p>
            {t('compareSavingFirst', {
              amount: money(comparison.savings_first_try, 'TRY'),
            })}
          </p>
          <p>
            {t('compareSavingRepeat', {
              amount: money(comparison.savings_repeat_try, 'TRY'),
            })}
          </p>
        </div>

        <dl className='space-y-2 border-t pt-4'>
          <ComparisonRow
            label={t('compareSource')}
            value={<SourceValue value={comparison.price_source} />}
          />
          <ComparisonRow
            label={t('comparePriceDate')}
            value={formatHwStamp(comparison.price_fetched_at)}
          />
          <ComparisonRow
            label={t('compareFxRate')}
            value={formatHwMoney(
              parseHwAmount(comparison.try_per_usd),
              'TRY',
              props.locale
            )}
          />
          <ComparisonRow
            label={t('compareFxSource')}
            value={<SourceValue value={comparison.fx_source} />}
          />
          <ComparisonRow
            label={t('compareFxDate')}
            value={formatHwStamp(comparison.fx_date)}
          />
        </dl>

        {comparison.prices_include_kdv && (
          <p className='text-muted-foreground text-sm'>{t('compareKdv')}</p>
        )}
      </CardContent>
    </Card>
  )
}
