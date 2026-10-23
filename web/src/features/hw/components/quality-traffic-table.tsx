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

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useIsMobile } from '@/hooks/use-mobile'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { formatHwStamp } from '../lib/format'
import { useHwText } from '../texts'
import type { HwQualityTraffic, HwQualityTrafficModel } from '../types'

type HwText = ReturnType<typeof useHwText>

function seconds(value: number | null, t: HwText, locale: string | undefined) {
  if (value == null) return '-'
  if (value === 0) return t('qualityUnderSecond')
  return t('qualitySecondsUnit', { value: formatNumber(value, locale) })
}

type Figure = { label: string; value: string }

/** The four measured figures of a model with enough data, in column order. */
function measuredFigures(
  entry: HwQualityTrafficModel,
  t: HwText,
  locale: string | undefined
): Figure[] {
  return [
    {
      label: t('qualitySuccess'),
      value:
        entry.success_percent == null
          ? '-'
          : t('qualityPercentUnit', {
              value: formatNumber(entry.success_percent, locale),
            }),
    },
    {
      label: t('qualityMedian'),
      value: seconds(entry.median_seconds, t, locale),
    },
    { label: t('qualityP95'), value: seconds(entry.p95_seconds, t, locale) },
    {
      label: t('qualitySpeed'),
      value:
        entry.output_tokens_per_second == null
          ? '-'
          : t('qualitySpeedUnit', {
              value: formatNumber(entry.output_tokens_per_second, locale),
            }),
    },
  ]
}

function useLocale() {
  const { i18n } = useTranslation()
  return toIntlLocale(i18n.resolvedLanguage || i18n.language)
}

function ModelRow(props: {
  entry: HwQualityTrafficModel
  minRequests: number
}) {
  const t = useHwText()
  const locale = useLocale()
  const entry = props.entry
  return (
    <TableRow>
      <TableCell className='font-mono text-xs sm:text-sm'>
        {entry.model}
      </TableCell>
      <TableCell>{formatNumber(entry.requests, locale)}</TableCell>
      {entry.enough_data ? (
        measuredFigures(entry, t, locale).map((figure) => (
          <TableCell key={figure.label}>{figure.value}</TableCell>
        ))
      ) : (
        <TableCell colSpan={4} className='text-muted-foreground'>
          {t('qualityTooFew', {
            min: formatNumber(props.minRequests, locale),
          })}
        </TableCell>
      )}
    </TableRow>
  )
}

/** Phones: one card per model with every figure in view, no sideways scroll. */
function ModelCard(props: {
  entry: HwQualityTrafficModel
  minRequests: number
}) {
  const t = useHwText()
  const locale = useLocale()
  const entry = props.entry
  const figures: Figure[] = [
    {
      label: t('qualityRequests'),
      value: formatNumber(entry.requests, locale),
    },
    ...(entry.enough_data ? measuredFigures(entry, t, locale) : []),
  ]
  return (
    <li
      className='bg-background/60 rounded-lg border p-4'
      data-testid='hw-quality-traffic-card'
    >
      <p className='font-mono text-sm font-semibold break-all'>{entry.model}</p>
      <dl className='mt-3 grid grid-cols-2 gap-x-4 gap-y-3'>
        {figures.map((figure) => (
          <div key={figure.label}>
            <dt className='text-muted-foreground text-xs'>{figure.label}</dt>
            <dd className='font-semibold tabular-nums'>{figure.value}</dd>
          </div>
        ))}
      </dl>
      {entry.enough_data ? null : (
        <p className='text-muted-foreground mt-3 text-sm'>
          {t('qualityTooFew', {
            min: formatNumber(props.minRequests, locale),
          })}
        </p>
      )}
    </li>
  )
}

function ModelTable(props: {
  models: HwQualityTrafficModel[]
  minRequests: number
}) {
  const t = useHwText()
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('qualityModelColumn')}</TableHead>
          <TableHead>{t('qualityRequests')}</TableHead>
          <TableHead>{t('qualitySuccess')}</TableHead>
          <TableHead>{t('qualityMedian')}</TableHead>
          <TableHead>{t('qualityP95')}</TableHead>
          <TableHead>{t('qualitySpeed')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.models.map((entry) => (
          <ModelRow
            key={entry.model}
            entry={entry}
            minRequests={props.minRequests}
          />
        ))}
      </TableBody>
    </Table>
  )
}

/**
 * The observed quality of every own model, one tab per window. Every figure
 * comes from the server; a model below the minimum shows only its count.
 */
export function QualityTrafficTable(props: { traffic: HwQualityTraffic }) {
  const t = useHwText()
  const locale = useLocale()
  const isMobile = useIsMobile()
  const windows = props.traffic.windows
  const first = windows[0]
  if (!first) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('qualityTrafficTitle')}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-4'>
        <Tabs defaultValue={String(first.days)}>
          <TabsList aria-label={t('qualityWindowsLabel')}>
            {windows.map((window) => (
              <TabsTrigger key={window.days} value={String(window.days)}>
                {t('qualityWindowTab', { days: String(window.days) })}
              </TabsTrigger>
            ))}
          </TabsList>
          {windows.map((window) => (
            <TabsContent key={window.days} value={String(window.days)}>
              {window.models.length === 0 ? (
                <p className='text-muted-foreground py-6 text-sm'>
                  {t('qualityWindowEmpty')}
                </p>
              ) : null}
              {window.models.length > 0 && isMobile ? (
                <ul className='space-y-3'>
                  {window.models.map((entry) => (
                    <ModelCard
                      key={entry.model}
                      entry={entry}
                      minRequests={props.traffic.min_requests}
                    />
                  ))}
                </ul>
              ) : null}
              {window.models.length > 0 && !isMobile ? (
                <ModelTable
                  models={window.models}
                  minRequests={props.traffic.min_requests}
                />
              ) : null}
            </TabsContent>
          ))}
        </Tabs>
        <p className='text-muted-foreground text-sm'>
          {t('qualityTrafficNote', {
            minutes: formatNumber(
              Math.max(1, Math.round(props.traffic.refresh_seconds / 60)),
              locale
            ),
            stamp: formatHwStamp(props.traffic.computed_at),
          })}
        </p>
      </CardContent>
    </Card>
  )
}
