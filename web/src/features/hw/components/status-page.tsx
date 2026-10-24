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
import { useQuery } from '@tanstack/react-query'
import {
  CircleCheck,
  CircleX,
  ShieldCheck,
  TriangleAlert,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getHwStatus } from '../api'
import { useSingleRetry } from '../hooks/use-single-retry'
import {
  formatHwStamp,
  hwStatusMessage,
  hwStatusTextKey,
} from '../lib/format'
import { httpStatus } from '../lib/http-status'
import { pickHwLanguage, useHwText, type HwTextKey } from '../texts'
import type { HwStatusState } from '../types'
import { StatusIncidents } from './status-incidents'

/** Colour, icon and wording of each overall state; the wording never relies on colour alone. */
const OVERALL_VIEW: Record<
  HwStatusState,
  { icon: LucideIcon; title: HwTextKey; body: HwTextKey; tone: string }
> = {
  operational: {
    icon: CircleCheck,
    title: 'statusOverallOperational',
    body: 'statusOverallOperationalBody',
    tone: 'border-success/30 bg-success/10 text-success',
  },
  degraded: {
    icon: TriangleAlert,
    title: 'statusOverallDegraded',
    body: 'statusOverallDegradedBody',
    tone: 'border-warning/40 bg-warning/10 text-warning',
  },
  outage: {
    icon: CircleX,
    title: 'statusOverallOutage',
    body: 'statusOverallOutageBody',
    tone: 'border-destructive/40 bg-destructive/10 text-destructive',
  },
}

const STATE_DOT: Record<string, string> = {
  operational: 'bg-success',
  degraded: 'bg-warning',
  outage: 'bg-destructive',
}

/** One model state as a badge; the label is the text, the dot only supports it. */
function ModelStateBadge(props: {
  state: string
  text: (key: HwTextKey) => string
}) {
  return (
    <span className='inline-flex items-center gap-1.5 text-sm font-medium'>
      <span
        className={cn(
          'size-2 shrink-0 rounded-full',
          STATE_DOT[props.state] ?? STATE_DOT.operational
        )}
        aria-hidden='true'
      />
      {props.text(hwStatusTextKey(props.state))}
    </span>
  )
}

/** `/durum`: the public service status, readable without a login. */
export function StatusPage(props: { retryDelayMs?: number } = {}) {
  const text = useHwText()
  const { i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const language = pickHwLanguage(i18n.resolvedLanguage || i18n.language)

  const status = useQuery({
    queryKey: ['hw-status'],
    queryFn: getHwStatus,
    refetchInterval: 60_000,
    staleTime: 30_000,
    meta: { errorToast: false },
  })

  useSingleRetry(
    () => {
      void status.refetch()
    },
    status.isError,
    props.retryDelayMs ?? 5000
  )

  const answer = status.data?.data
  const models = answer?.models ?? []
  const overall = OVERALL_VIEW[answer?.overall ?? 'operational']
  const OverallIcon = overall.icon

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition className='container mx-auto max-w-4xl space-y-6 px-4 pt-24 pb-12'>
        <header className='space-y-2'>
          <h1 className='text-3xl font-semibold tracking-tight'>
            {text('statusTitle')}
          </h1>
          <p className='text-muted-foreground'>{text('statusLead')}</p>
        </header>

        {status.isLoading && (
          <div className='space-y-3'>
            <p className='text-muted-foreground'>{text('statusLoading')}</p>
            <Skeleton className='h-24 w-full rounded-xl' />
            <Skeleton className='h-48 w-full rounded-xl' />
          </div>
        )}

        {status.isError && (
          <Alert variant='destructive'>
            <AlertTitle>{text('statusTitle')}</AlertTitle>
            <AlertDescription>
              <p>
                {httpStatus(status.error) === 429
                  ? text('rateLimited')
                  : text('statusError')}
              </p>
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='mt-3'
                onClick={() => {
                  void status.refetch()
                }}
              >
                {text('retry')}
              </Button>
            </AlertDescription>
          </Alert>
        )}

        {answer && (
          <section
            aria-labelledby='hw-status-overall'
            className={cn('rounded-xl border p-5', overall.tone)}
          >
            <h2
              id='hw-status-overall'
              className='flex items-center gap-2 text-lg font-semibold'
            >
              <OverallIcon className='size-5 shrink-0' aria-hidden='true' />
              {text(overall.title)}
            </h2>
            <p className='mt-1.5 text-sm'>{text(overall.body)}</p>
          </section>
        )}

        {answer && models.length === 0 && (
          <EmptyState
            bordered
            description={text('statusEmptyBody')}
            title={text('statusEmpty')}
          />
        )}

        {answer && models.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle>{text('statusModelsTitle')}</CardTitle>
            </CardHeader>
            <CardContent>
              <p className='text-muted-foreground mb-2 text-xs sm:hidden'>
                {text('statusScrollHint')}
              </p>
              <div className='overflow-x-auto'>
                <Table withContainer={false} className='min-w-[34rem]'>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{text('statusModelColumn')}</TableHead>
                      <TableHead>{text('statusColumn')}</TableHead>
                      <TableHead>
                        {text('statusAvailabilityColumn', {
                          days: answer.availability_days,
                        })}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {models.map((entry) => (
                      <TableRow key={entry.model}>
                        <TableCell>
                          <span className='font-medium'>{entry.model}</span>
                          <span className='text-muted-foreground block text-xs'>
                            {hwStatusMessage(entry, language)}
                          </span>
                        </TableCell>
                        <TableCell>
                          <ModelStateBadge state={entry.status} text={text} />
                        </TableCell>
                        <TableCell>
                          {entry.availability_percent > 0
                            ? text('statusAvailabilityValue', {
                                percent: formatNumber(
                                  entry.availability_percent,
                                  locale
                                ),
                              })
                            : text('statusNoMeasurement')}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
          </Card>
        )}

        {answer && (
          <section aria-labelledby='hw-status-incidents' className='space-y-3'>
            <h2 id='hw-status-incidents' className='text-xl font-semibold'>
              {text('statusIncidentsTitle')}
            </h2>
            <StatusIncidents
              incident={answer.incident}
              language={language}
              recent={answer.recent ?? []}
              text={text}
            />
          </section>
        )}

        {answer && (
          <Card>
            <CardHeader>
              <CardTitle className='flex items-center gap-2 text-base'>
                <ShieldCheck className='size-4' aria-hidden='true' />
                {text('statusExtensionTitle')}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <p className='text-muted-foreground text-sm'>
                {text('statusExtensionBody')}
              </p>
            </CardContent>
          </Card>
        )}

        {answer && (
          <p className='text-muted-foreground text-sm'>
            {text('statusUpdatedAt', {
              time: formatHwStamp(answer.checked_at),
            })}
            {' · '}
            {text('statusAutoRefresh')}
          </p>
        )}
      </PageTransition>
    </PublicLayout>
  )
}
