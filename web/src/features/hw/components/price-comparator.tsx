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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { PageTransition } from '@/components/page-transition'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { getServerErrorStatus } from '@/lib/server-error-message'

import { compareHwPrices, type HwCompareRequest } from '../api'
import { HW_COMPARE_MAX_MONTHLY_TOKENS, HW_SOLD_MODELS } from '../constants'
import { parseHwTokenCount } from '../lib/format'
import { useHwText } from '../texts'
import { ComparisonResult } from './comparison-result'
import { PriceTable } from './price-table'

export function PriceComparator() {
  const t = useHwText()
  const { i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  const [model, setModel] = useState<string>(HW_SOLD_MODELS[0])
  const [inputText, setInputText] = useState('')
  const [outputText, setOutputText] = useState('')
  const [invalid, setInvalid] = useState(false)
  const [request, setRequest] = useState<HwCompareRequest | null>(null)

  const comparison = useQuery({
    queryKey: ['hw-price-comparison', request],
    queryFn: () => compareHwPrices(request as HwCompareRequest),
    enabled: request !== null,
    staleTime: 5 * 60 * 1000,
    meta: { errorToast: false },
  })

  const handleSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const inputTokens = parseHwTokenCount(inputText)
    const outputTokens = parseHwTokenCount(outputText)
    const bothZero = inputTokens === 0 && outputTokens === 0
    if (inputTokens === null || outputTokens === null || bothZero) {
      setInvalid(true)
      setRequest(null)
      return
    }
    setInvalid(false)
    setRequest({
      model,
      input_tokens: inputTokens,
      output_tokens: outputTokens,
    })
  }

  const tokenHint = t('compareTokensNote', {
    max: formatNumber(HW_COMPARE_MAX_MONTHLY_TOKENS, locale),
  })
  const notFound = getServerErrorStatus(comparison.error) === 404

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition className='container mx-auto max-w-6xl space-y-6 px-4 pt-24 pb-12'>
        <header className='space-y-2'>
          <h1 className='text-3xl font-semibold tracking-tight'>
            {t('compareTitle')}
          </h1>
          <p className='text-muted-foreground'>{t('compareLead')}</p>
        </header>

        <PriceTable />

        <Card>
          <CardHeader>
            <CardTitle>{t('compareTitle')}</CardTitle>
          </CardHeader>
          <CardContent>
            <form className='space-y-4' noValidate onSubmit={handleSubmit}>
              <div className='space-y-2'>
                <Label htmlFor='hw-compare-model'>
                  {t('compareModelLabel')}
                </Label>
                <Select
                  value={model}
                  onValueChange={(value) => {
                    if (value) setModel(value)
                  }}
                >
                  <SelectTrigger className='w-full' id='hw-compare-model'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {HW_SOLD_MODELS.map((name) => (
                      <SelectItem key={name} value={name}>
                        {name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className='grid gap-4 sm:grid-cols-2'>
                <div className='space-y-2'>
                  <Label htmlFor='hw-compare-input'>
                    {t('compareInputLabel')}
                  </Label>
                  <Input
                    id='hw-compare-input'
                    inputMode='numeric'
                    placeholder='1000000'
                    value={inputText}
                    onChange={(event) => setInputText(event.target.value)}
                  />
                </div>
                <div className='space-y-2'>
                  <Label htmlFor='hw-compare-output'>
                    {t('compareOutputLabel')}
                  </Label>
                  <Input
                    id='hw-compare-output'
                    inputMode='numeric'
                    placeholder='250000'
                    value={outputText}
                    onChange={(event) => setOutputText(event.target.value)}
                  />
                </div>
              </div>

              <p className='text-muted-foreground text-sm'>{tokenHint}</p>

              <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                <Button disabled={comparison.isFetching} type='submit'>
                  {t('compareSubmit')}
                </Button>
                <p className='text-muted-foreground text-sm'>
                  {t('compareKdv')}
                </p>
              </div>
            </form>
          </CardContent>
        </Card>

        {invalid && (
          <Alert variant='destructive'>
            <AlertTitle>{t('compareEmptyTitle')}</AlertTitle>
            <AlertDescription>{tokenHint}</AlertDescription>
          </Alert>
        )}

        {request && comparison.isPending && (
          <div className='space-y-3'>
            <p className='text-muted-foreground'>{t('compareLoading')}</p>
            <Skeleton className='h-64 w-full rounded-xl' />
          </div>
        )}

        {request && comparison.isError && (
          <Alert variant='destructive'>
            <AlertTitle>{t('compareTitle')}</AlertTitle>
            <AlertDescription>
              {notFound ? t('compareErrorModel') : t('compareErrorDefault')}
            </AlertDescription>
          </Alert>
        )}

        {comparison.data && (
          <ComparisonResult comparison={comparison.data.data} locale={locale} />
        )}

        {!request && !invalid && (
          <p className='text-muted-foreground'>{t('compareEmptyTitle')}</p>
        )}
      </PageTransition>
      <Footer />
    </PublicLayout>
  )
}
