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
import { memo, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Hint } from '@/features/hw/components/hint'
import {
  formatLatency,
  getSuccessRateDotClass,
} from '@/features/performance-metrics/lib/format'
import type { SuccessRatePoint } from '@/features/performance-metrics/types'
import { cn } from '@/lib/utils'

import { wordsPerSecond } from '../lib/plain-metrics'

export type ModelPerfBadgeData = {
  window_start?: number
  window_end?: number
  avg_latency_ms: number
  success_rate: number
  avg_tps: number
  recent_success_series?: SuccessRatePoint[]
}

export interface ModelPerfBadgeProps extends React.HTMLAttributes<HTMLDivElement> {
  perf: ModelPerfBadgeData | undefined
}

const STATUS_SAMPLE_LIMIT = 24

export const ModelPerfBadge = memo(function ModelPerfBadge(
  props: ModelPerfBadgeProps
) {
  const { t } = useTranslation()
  const latencyText = formatLatency(props.perf?.avg_latency_ms ?? 0)
  const words = wordsPerSecond(props.perf?.avg_tps ?? 0)
  const successRate = props.perf?.success_rate
  const hasSuccessRate =
    successRate != null &&
    Number.isFinite(successRate) &&
    successRate >= 0 &&
    successRate <= 100
  const statusSamples = useMemo(
    () =>
      (props.perf?.recent_success_series ?? [])
        .filter(
          (point) =>
            Number.isFinite(point.ts) &&
            Number.isFinite(point.success_rate) &&
            point.success_rate >= 0 &&
            point.success_rate <= 100
        )
        .sort((a, b) => a.ts - b.ts)
        .slice(-STATUS_SAMPLE_LIMIT),
    [props.perf?.recent_success_series]
  )
  const statusLabel =
    statusSamples.length > 0
      ? t('Uptime (last 24 hours)')
      : t('No performance data available')

  return (
    <div
      aria-label={t('Performance metrics for the last 24 hours')}
      className={cn(
        'flex w-full min-w-0 flex-wrap items-center justify-between gap-x-3 gap-y-2',
        props.className
      )}
    >
      <dl className='flex min-w-0 flex-wrap items-start gap-x-5 gap-y-2 text-xs tabular-nums'>
        <div className='shrink-0'>
          <dt className='text-muted-foreground text-[11px] leading-4'>
            <Hint
              hint={t(
                'How often requests to this model worked in the last 24 hours. Each bar is one hour; green means everything worked.'
              )}
            >
              {t('Uptime (last 24 hours)')}
            </Hint>
          </dt>
          <dd className='mt-1 flex items-center gap-2'>
            <span
              role='img'
              aria-label={statusLabel}
              title={statusLabel}
              className='flex h-3 w-24 items-center gap-px'
            >
              {statusSamples.length > 0 ? (
                statusSamples.map((sample) => (
                  <span
                    key={sample.ts}
                    aria-hidden
                    className={cn(
                      'h-full w-[3px] shrink-0 rounded-xs',
                      getSuccessRateDotClass(sample.success_rate)
                    )}
                  />
                ))
              ) : (
                <span className='text-muted-foreground'>—</span>
              )}
            </span>
            <span className='font-mono'>
              {hasSuccessRate ? `${successRate.toFixed(2)}%` : '—'}
            </span>
          </dd>
        </div>
        <div className='shrink-0'>
          <dt className='text-muted-foreground text-[11px] leading-4'>
            <Hint
              hint={t(
                'The average time a request takes to come back. Lower is faster.'
              )}
            >
              {t('Response time')}
            </Hint>
          </dt>
          <dd className='mt-1 font-mono whitespace-nowrap'>
            {latencyText === '—' ? '—s' : latencyText}
          </dd>
        </div>
        <div className='shrink-0'>
          <dt className='text-muted-foreground text-[11px] leading-4'>
            <Hint
              hint={t(
                'How fast the model writes its answer. A fast model fills the screen quickly; a slow one types word by word.'
              )}
            >
              {t('Writing speed')}
            </Hint>
          </dt>
          <dd className='mt-1 whitespace-nowrap'>
            {words === null ? '—' : t('~{{words}} words per second', { words })}
          </dd>
        </div>
      </dl>
      {props.children}
    </div>
  )
})
