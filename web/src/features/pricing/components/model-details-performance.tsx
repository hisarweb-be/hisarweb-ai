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
import { AlertTriangle, HeartPulse, Timer } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  StaticDataTable,
  staticDataTableClassNames as tableStyles,
} from '@/components/data-table'
import { Hint } from '@/features/hw/components/hint'
import { getPerfMetrics } from '@/features/performance-metrics/api'
import {
  formatLatency,
  formatThroughput,
  formatUptimePct,
  getSuccessRateTextClass,
} from '@/features/performance-metrics/lib/format'
import type {
  PerformanceGroup,
  PerformanceSeriesPoint,
} from '@/features/performance-metrics/types'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import type { UptimeDayPoint } from '../lib/mock-stats'
import {
  averageResponseTimeMs,
  plainGroupLabel,
  wordsPerSecond,
} from '../lib/plain-metrics'
import type { PricingModel } from '../types'
import { LatencyTrendChart, UptimeTrendChart } from './model-details-charts'
import { UptimeSparkline } from './model-details-uptime-sparkline'

function StatCard(props: {
  icon: React.ComponentType<{ className?: string }>
  label: React.ReactNode
  value: React.ReactNode
  hint?: string
  valueClassName?: string
}) {
  const Icon = props.icon
  return (
    <div className='bg-background flex flex-col gap-1 rounded-lg border p-3'>
      <span className='text-muted-foreground inline-flex items-center gap-1.5 text-[10px] font-medium tracking-wider uppercase'>
        <Icon className='size-3' />
        {props.label}
      </span>
      <span
        className={cn(
          'text-foreground font-mono text-lg font-semibold tabular-nums',
          props.valueClassName
        )}
      >
        {props.value}
      </span>
      {props.hint && (
        <span className='text-muted-foreground/70 text-[11px]'>
          {props.hint}
        </span>
      )}
    </div>
  )
}

type PerformanceRow = {
  group: string
  avg_ttft_ms: number
  avg_latency_ms: number
  success_rate: number
  avg_tps: number
}

function toUptimePct(value: number): number {
  if (!Number.isFinite(value)) return 0
  const clamped = Math.min(100, Math.max(0, value))
  return Math.round(clamped * 100) / 100
}

function toLatencySeries(series: PerformanceSeriesPoint[]) {
  return series
    .filter((point) => point.avg_ttft_ms > 0)
    .map((point) => ({
      timestamp: new Date(point.ts * 1000).toISOString(),
      group: 'latency',
      ttft_ms: point.avg_ttft_ms,
    }))
}

function toUptimeSeries(series: PerformanceSeriesPoint[]): UptimeDayPoint[] {
  return series.map((point) => ({
    date: new Date(point.ts * 1000).toISOString(),
    uptime_pct: toUptimePct(point.success_rate),
    incidents: point.success_rate < 100 ? 1 : 0,
    outage_minutes: 0,
  }))
}

function toGroupUptimeSeries(group: PerformanceGroup): UptimeDayPoint[] {
  return group.series.map((point) => {
    const successRate = toUptimePct(point.success_rate)
    return {
      date: new Date(point.ts * 1000).toISOString(),
      uptime_pct: successRate,
      incidents: successRate < 100 ? 1 : 0,
      outage_minutes: 0,
    }
  })
}

export function ModelDetailsPerformance(props: { model: PricingModel }) {
  const { t } = useTranslation()
  const metricsQuery = useQuery({
    queryKey: ['perf-metrics', props.model.model_name],
    queryFn: async () =>
      requireServerSuccess(await getPerfMetrics(props.model.model_name, 24)),
    staleTime: 60 * 1000,
  })
  const groups = useMemo(
    () => metricsQuery.data?.data.groups ?? [],
    [metricsQuery.data]
  )
  const summary = metricsQuery.data?.data.summary
  const series = useMemo(
    () => metricsQuery.data?.data.series ?? [],
    [metricsQuery.data]
  )
  const performances = useMemo<PerformanceRow[]>(
    () =>
      groups.map((group) => ({
        group: group.group,
        avg_ttft_ms: group.avg_ttft_ms,
        avg_latency_ms: group.avg_latency_ms,
        success_rate: group.success_rate,
        avg_tps: group.avg_tps,
      })),
    [groups]
  )
  const latencySeries = useMemo(() => toLatencySeries(series), [series])
  const uptimeSeries = useMemo(() => toUptimeSeries(series), [series])
  const uptimeByGroup = useMemo<Record<string, UptimeDayPoint[]>>(() => {
    const map: Record<string, UptimeDayPoint[]> = {}
    for (const group of groups) {
      map[group.group] = toGroupUptimeSeries(group)
    }
    return map
  }, [groups])

  if (metricsQuery.isLoading || performances.length === 0) {
    return (
      <div className='text-muted-foreground rounded-lg border p-6 text-center text-sm'>
        {t('Performance data is not yet available for this model.')}
      </div>
    )
  }

  const avgTps = summary?.avg_tps ?? 0
  const avgTtft = averageResponseTimeMs(groups)
  const successRate = summary?.success_rate ?? Number.NaN
  const incidentCount = uptimeSeries.reduce((s, p) => s + p.incidents, 0)
  const writingSpeed = (tokensPerSecond: number) => {
    const words = wordsPerSecond(tokensPerSecond)
    return words === null
      ? formatThroughput(tokensPerSecond)
      : t('~{{words}} words per second', { words })
  }
  const writingSpeedHint = t(
    'How fast the model writes its answer. A word is about 1.3 tokens.'
  )
  const responseTimeHint = t(
    'How long you wait until the first words of the answer appear.'
  )
  const uptimeHint = t(
    'The share of requests that worked in the last 24 hours. Higher is better.'
  )

  return (
    <div className='flex flex-col gap-4'>
      <div className='grid grid-cols-1 gap-2 sm:grid-cols-3'>
        <StatCard
          icon={Timer}
          label={<Hint hint={writingSpeedHint}>{t('Writing speed')}</Hint>}
          value={writingSpeed(avgTps)}
        />
        <StatCard
          icon={Timer}
          label={<Hint hint={responseTimeHint}>{t('Response time')}</Hint>}
          value={formatLatency(avgTtft)}
        />
        <StatCard
          icon={HeartPulse}
          label={<Hint hint={uptimeHint}>{t('Uptime (last 24 hours)')}</Hint>}
          value={formatUptimePct(successRate)}
          hint={
            incidentCount > 0
              ? t('{{count}} incidents in the last 24 hours', {
                  count: incidentCount,
                })
              : t('No incidents in the last 24 hours')
          }
          valueClassName={getSuccessRateTextClass(successRate)}
        />
      </div>

      <section>
        <SectionHeader
          icon={HeartPulse}
          title={t('Performance')}
          description={t(
            'Average response time, writing speed and uptime over the last 24 hours'
          )}
        />
        <StaticDataTable
          className='rounded-lg'
          tableClassName='text-sm'
          headerRowClassName={tableStyles.compactHeaderRow}
          data={performances}
          getRowKey={(perf) => perf.group}
          columns={[
            {
              id: 'group',
              header: t('Available to'),
              className: tableStyles.compactHeaderCell,
              cellClassName: tableStyles.compactCell,
              cell: (perf) => plainGroupLabel(perf.group, t),
            },
            {
              id: 'tps',
              header: <Hint hint={writingSpeedHint}>{t('Writing speed')}</Hint>,
              className: tableStyles.compactHeaderCellRight,
              cellClassName: tableStyles.compactNumericCell,
              cell: (perf) => writingSpeed(perf.avg_tps),
            },
            {
              id: 'ttft',
              header: <Hint hint={responseTimeHint}>{t('Response time')}</Hint>,
              className: tableStyles.compactHeaderCellRight,
              cellClassName: tableStyles.compactNumericCell,
              cell: (perf) => formatLatency(perf.avg_ttft_ms),
            },
            {
              id: 'latency',
              header: (
                <Hint
                  hint={t(
                    'How long the whole answer takes, from sending your request until the last word.'
                  )}
                >
                  {t('Full answer time')}
                </Hint>
              ),
              className: tableStyles.compactHeaderCellRight,
              cellClassName: tableStyles.compactMutedNumericCell,
              cell: (perf) => formatLatency(perf.avg_latency_ms),
            },
            {
              id: 'success',
              header: (
                <Hint hint={uptimeHint}>{t('Uptime (last 24 hours)')}</Hint>
              ),
              className: cn(tableStyles.compactHeaderCell, 'min-w-[180px]'),
              cellClassName: tableStyles.compactCell,
              cell: (perf) => (
                <UptimeSparkline
                  size='sm'
                  series={uptimeByGroup[perf.group] ?? []}
                  overallSuccessRate={perf.success_rate}
                />
              ),
            },
          ]}
        />
      </section>

      <section>
        <SectionHeader
          icon={Timer}
          title={t('Response time (last 24 hours)')}
          description={t(
            'How long you wait for the first words, hour by hour.'
          )}
        />
        <LatencyTrendChart series={latencySeries} />
      </section>

      <section>
        <SectionHeader
          icon={HeartPulse}
          title={t('Uptime (last 24 hours)')}
          description={t('The share of requests that worked, hour by hour.')}
          accent={
            incidentCount > 0 ? (
              <span className='inline-flex items-center gap-1 text-amber-600 dark:text-amber-400'>
                <AlertTriangle className='size-3.5' />
                {t('{{count}} incidents', {
                  count: incidentCount,
                })}
              </span>
            ) : null
          }
        />
        <UptimeTrendChart series={uptimeSeries} />
      </section>
    </div>
  )
}

function SectionHeader(props: {
  icon: React.ComponentType<{ className?: string }>
  title: string
  description?: string
  accent?: React.ReactNode
}) {
  const Icon = props.icon
  return (
    <div className='mb-2 flex flex-wrap items-center justify-between gap-2'>
      <div className='flex min-w-0 items-center gap-2'>
        <Icon className='text-muted-foreground/70 size-3.5 shrink-0' />
        <div className='min-w-0'>
          <div className='text-foreground text-sm font-semibold'>
            {props.title}
          </div>
          {props.description && (
            <p className='text-muted-foreground/80 text-xs'>
              {props.description}
            </p>
          )}
        </div>
      </div>
      {props.accent && (
        <div className='shrink-0 text-xs font-medium'>{props.accent}</div>
      )}
    </div>
  )
}
