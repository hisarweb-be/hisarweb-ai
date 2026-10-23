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
import { useTranslation } from 'react-i18next'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toIntlLocale } from '@/i18n/languages'
import { cn } from '@/lib/utils'

import '@/styles/hw-cinematic.css'

import {
  MODEL_PRICES,
  MODEL_PRICES_FX_DATE,
  MODEL_PRICES_TRY_PER_USD,
  PRICE_SOURCES,
  savingFraction,
  type ModelPrice,
  type TokenPrice,
} from '../data/model-prices'
import { usePriceTableText } from '../price-table-texts'

type PriceTableProps = {
  models?: readonly ModelPrice[]
  tryPerUsd?: number
}

type Translate = ReturnType<typeof usePriceTableText>

// The model name stays in view while a phone scrolls sideways to the savings.
const STICKY_MODEL = 'bg-background sticky left-0 z-10 border-r'

function formatMoney(
  value: number,
  currency: 'TRY' | 'USD',
  locale: string | undefined
): string {
  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency,
    currencyDisplay: 'narrowSymbol',
    minimumFractionDigits: 2,
    maximumFractionDigits: currency === 'USD' ? 3 : 2,
  }).format(value)
}

function formatPercent(fraction: number, locale: string | undefined): string {
  return new Intl.NumberFormat(locale, {
    style: 'percent',
    maximumFractionDigits: 1,
  }).format(fraction)
}

/** One amount in dollars and in lira; a missing price stays a dash. */
function AmountCell(props: {
  usd: number | undefined
  tryPerUsd: number
  locale: string | undefined
  missing: string
}) {
  if (props.usd === undefined) {
    return (
      <TableCell className='text-muted-foreground'>{props.missing}</TableCell>
    )
  }
  return (
    <TableCell>
      <div>{formatMoney(props.usd, 'USD', props.locale)}</div>
      <div className='text-muted-foreground' data-table-text='secondary'>
        {formatMoney(props.usd * props.tryPerUsd, 'TRY', props.locale)}
      </div>
    </TableCell>
  )
}

function PriceCells(props: {
  price: TokenPrice | null
  tryPerUsd: number
  locale: string | undefined
  missing: string
}) {
  return (
    <>
      <AmountCell
        locale={props.locale}
        missing={props.missing}
        tryPerUsd={props.tryPerUsd}
        usd={props.price?.input}
      />
      <AmountCell
        locale={props.locale}
        missing={props.missing}
        tryPerUsd={props.tryPerUsd}
        usd={props.price?.output}
      />
    </>
  )
}

/**
 * A saving is a pill with the percentage; a price above the reference is
 * "no saving". Both saving columns carry the best-price highlight, the first
 * one also the edge that opens the highlighted band.
 */
function SavingCell(props: {
  fraction: number | null
  locale: string | undefined
  t: Translate
  edge?: boolean
}) {
  const fraction = props.fraction
  const best = cn('hwp-best', props.edge && 'hwp-best-edge')
  if (fraction === null) {
    return (
      <TableCell className={cn(best, 'text-muted-foreground')}>
        {props.t('missing')}
      </TableCell>
    )
  }
  if (Math.abs(fraction) < 1e-9) {
    return <TableCell className={best}>{props.t('savingNoneZero')}</TableCell>
  }
  if (fraction > 0) {
    return (
      <TableCell className={best}>
        <span className='hwp-best-pill' data-testid='hw-saving-pill'>
          {props.t('savingPill', {
            percent: formatPercent(fraction, props.locale),
          })}
        </span>
      </TableCell>
    )
  }
  return (
    <TableCell className={cn(best, 'text-destructive')}>
      <div>{props.t('savingNone')}</div>
      <div data-table-text='secondary'>
        {props.t('savingNoneMore', {
          percent: formatPercent(Math.abs(fraction), props.locale),
        })}
      </div>
    </TableCell>
  )
}

function PriceRow(props: {
  row: ModelPrice
  tryPerUsd: number
  locale: string | undefined
  t: Translate
}) {
  const row = props.row
  const common = {
    tryPerUsd: props.tryPerUsd,
    locale: props.locale,
    missing: props.t('missing'),
  }
  return (
    <TableRow>
      <TableCell className={STICKY_MODEL}>
        <div className='font-semibold'>{row.model}</div>
        <div className='text-muted-foreground' data-table-text='secondary'>
          {row.provider}
        </div>
      </TableCell>
      <PriceCells price={row.official} {...common} />
      <PriceCells price={row.openrouter} {...common} />
      <PriceCells price={row.hisarweb} {...common} />
      <SavingCell
        edge
        fraction={savingFraction(row.official, row.hisarweb)}
        locale={props.locale}
        t={props.t}
      />
      <SavingCell
        fraction={savingFraction(row.openrouter, row.hisarweb)}
        locale={props.locale}
        t={props.t}
      />
    </TableRow>
  )
}

function PriceTableHead(props: { t: Translate }) {
  const t = props.t
  const group = 'border-l text-center'
  return (
    <TableHeader>
      <TableRow className='hover:bg-transparent'>
        <TableHead className={STICKY_MODEL} rowSpan={2}>
          {t('colModel')}
        </TableHead>
        <TableHead className={group} colSpan={2}>
          {t('colOfficial')}
        </TableHead>
        <TableHead className={group} colSpan={2}>
          {t('colOpenRouter')}
        </TableHead>
        <TableHead className={group} colSpan={2}>
          {t('colHisarweb')}
        </TableHead>
        <TableHead
          className={cn(group, 'hwp-best hwp-best-edge hwp-best-head py-2')}
          colSpan={2}
        >
          <span className='hwp-best-label'>{t('bestPrice')}</span>
          <span className='block'>{t('colSaving')}</span>
        </TableHead>
      </TableRow>
      <TableRow className='hover:bg-transparent'>
        {[0, 1, 2].map((group) => (
          <PriceSubHeads key={group} t={t} />
        ))}
        <TableHead className='hwp-best hwp-best-edge hwp-best-head border-l'>
          {t('colVsOfficial')}
        </TableHead>
        <TableHead className='hwp-best hwp-best-head'>
          {t('colVsOpenRouter')}
        </TableHead>
      </TableRow>
    </TableHeader>
  )
}

function PriceSubHeads(props: { t: Translate }) {
  return (
    <>
      <TableHead className='border-l'>{props.t('colInput')}</TableHead>
      <TableHead>{props.t('colOutput')}</TableHead>
    </>
  )
}

/** Source list: only the sources that back at least one price on the table. */
function SourceList(props: { models: readonly ModelPrice[]; t: Translate }) {
  const providers = new Set(props.models.map((m) => m.provider.toLowerCase()))
  const sources = PRICE_SOURCES.filter(
    (source) =>
      source.id === 'openrouter' ||
      source.id === 'hisarweb' ||
      providers.has(source.id)
  )
  return (
    <section className='space-y-2'>
      <h3 className='text-sm font-semibold'>{props.t('sourcesTitle')}</h3>
      <ul className='text-muted-foreground space-y-1 text-sm'>
        {sources.map((source) => (
          <li className='break-words' key={source.id}>
            <span className='text-foreground font-medium'>{source.name}</span>
            {': '}
            <a
              className='text-primary underline underline-offset-2'
              href={source.url}
              rel='noreferrer noopener'
              target='_blank'
            >
              {source.url}
            </a>
            {' · '}
            {props.t('sourceDate', { date: source.measuredAt })}
          </li>
        ))}
      </ul>
    </section>
  )
}

export function PriceTable(props: PriceTableProps) {
  const t = usePriceTableText()
  const { i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const models = props.models ?? MODEL_PRICES
  const tryPerUsd = props.tryPerUsd ?? MODEL_PRICES_TRY_PER_USD

  return (
    <Card data-testid='hw-price-table'>
      <CardHeader>
        <CardTitle>
          <h2>{t('title')}</h2>
        </CardTitle>
        <CardDescription className='space-y-2'>
          <span className='block'>{t('introWhat')}</span>
          <span className='block'>{t('introToken')}</span>
          <span className='block'>{t('introWhy')}</span>
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-4'>
        <div
          aria-label={t('tableLabel')}
          className='max-w-full overflow-x-auto rounded-md border'
          role='region'
          tabIndex={0}
        >
          <Table withContainer={false}>
            <PriceTableHead t={t} />
            <TableBody>
              {models.map((row) => (
                <PriceRow
                  key={row.model}
                  locale={locale}
                  row={row}
                  t={t}
                  tryPerUsd={tryPerUsd}
                />
              ))}
            </TableBody>
          </Table>
        </div>

        <section className='space-y-2'>
          <h3 className='text-sm font-semibold'>{t('notesTitle')}</h3>
          <ul className='text-muted-foreground list-disc space-y-1 pl-5 text-sm'>
            <li>
              {t('noteFx', {
                rate: tryPerUsd.toLocaleString(locale),
                date: MODEL_PRICES_FX_DATE,
              })}
            </li>
            <li>{t('noteSaving')}</li>
            <li>{t('noteContext')}</li>
            <li>{t('noteOthers')}</li>
          </ul>
        </section>

        <SourceList models={models} t={t} />
      </CardContent>
    </Card>
  )
}
