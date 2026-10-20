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
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'

import { useHwPackages } from '../hooks/use-hw-packages'
import { useHwText } from '../texts'

import { CreditPackagesCard } from './credit-packages-card'
import { TopUpForm } from './top-up-form'

interface PurchasePageProps {
  /** True when the URL carries the payment provider's return parameters. */
  returnedFromPayment?: boolean
}

/**
 * Top-up page of the dashboard: pick a value, leave for the payment page, and
 * read the packages back on return. There is no route that reports the payment
 * state of an order, so the notice after returning only says that the balance
 * follows once the payment is confirmed.
 */
export function PurchasePage(props: PurchasePageProps) {
  const hw = useHwText()
  const { i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage ?? i18n.language)
  const packages = useHwPackages()
  const refetchPackages = packages.refetch

  useEffect(() => {
    if (props.returnedFromPayment) void refetchPackages()
  }, [props.returnedFromPayment, refetchPackages])

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{hw('topUpPageTitle')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-3xl flex-col gap-4'>
          {props.returnedFromPayment && (
            <Alert>
              <AlertTitle>{hw('topUpReturnTitle')}</AlertTitle>
              <AlertDescription>
                <p className='font-medium'>{hw('topUpReturnPending')}</p>
                <p className='text-muted-foreground text-sm'>
                  {hw('topUpReturnBody')}
                </p>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => void refetchPackages()}
                >
                  {hw('topUpRefresh')}
                </Button>
              </AlertDescription>
            </Alert>
          )}

          <TopUpForm locale={locale} />

          <CreditPackagesCard
            data={packages.data}
            isLoading={packages.isLoading}
            isError={packages.isError}
            onRefresh={() => void refetchPackages()}
            locale={locale}
          />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
