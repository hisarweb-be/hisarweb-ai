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
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { PageTransition } from '@/components/page-transition'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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

import { getHwQualityLatest } from '../api'
import { formatHwDate } from '../lib/format'
import { pickHwLanguage, useHwText } from '../texts'

export function QualityMeter() {
  const t = useHwText()
  const { i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const language = pickHwLanguage(i18n.resolvedLanguage || i18n.language)

  const quality = useQuery({
    queryKey: ['hw-quality-latest'],
    queryFn: getHwQualityLatest,
    staleTime: 10 * 60 * 1000,
    meta: { errorToast: false },
  })

  const summary = quality.data?.data
  const models = summary?.models ?? []
  const sampleNote =
    (language === 'tr' ? summary?.note : summary?.note_en || summary?.note) ||
    t('qualitySampleNote')

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition className='container mx-auto max-w-5xl space-y-6 px-4 pt-24 pb-12'>
        <header className='space-y-2'>
          <h1 className='text-3xl font-semibold tracking-tight'>
            {t('qualityTitle')}
          </h1>
          <p className='text-muted-foreground'>{t('qualityLead')}</p>
        </header>

        {quality.isLoading && (
          <div className='space-y-3'>
            <p className='text-muted-foreground'>{t('qualityLoading')}</p>
            <Skeleton className='h-64 w-full rounded-xl' />
          </div>
        )}

        {quality.isError && (
          <Alert variant='destructive'>
            <AlertTitle>{t('qualityTitle')}</AlertTitle>
            <AlertDescription>{t('qualityError')}</AlertDescription>
          </Alert>
        )}

        {summary && models.length === 0 && (
          <EmptyState
            bordered
            description={t('qualityEmptyBody')}
            title={t('qualityEmpty')}
          />
        )}

        {summary && models.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle>{t('qualityTitle')}</CardTitle>
              <p className='text-muted-foreground text-sm'>
                {t('qualityRunLine', {
                  tasks: formatNumber(summary.task_count, locale),
                  version: summary.taskset_version || '-',
                })}
              </p>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('qualityModelColumn')}</TableHead>
                    <TableHead>{t('qualityScore')}</TableHead>
                    <TableHead>{t('qualityLatency')}</TableHead>
                    <TableHead>{t('qualityMeasuredAt')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {models.map((entry) => (
                    <TableRow key={entry.model}>
                      <TableCell>{entry.model}</TableCell>
                      <TableCell>
                        {formatNumber(entry.avg_score, locale)}
                      </TableCell>
                      <TableCell>
                        {t('qualityLatencyUnit', {
                          ms: formatNumber(entry.avg_latency_ms, locale),
                        })}
                      </TableCell>
                      <TableCell>{formatHwDate(summary.measured_at)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        )}

        {summary && (
          <p className='text-muted-foreground text-sm'>{sampleNote}</p>
        )}
      </PageTransition>
      <Footer />
    </PublicLayout>
  )
}
