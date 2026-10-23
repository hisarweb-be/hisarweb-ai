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
import { Link } from '@tanstack/react-router'
import { CreditCard, Gift, Hourglass } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { TitledCard } from '@/components/ui/titled-card'
import { useStatus } from '@/hooks/use-status'

import type { HwTopUpLimits } from '../lib/currency'
import { useHwCurrency } from '../hooks/use-hw-currency'
import { useHwTopUpOrder } from '../hooks/use-hw-top-up-order'
import { hwOrderErrorTextKey, openCheckout } from '../lib/checkout'
import { formatHwAmount } from '../lib/format'
import { useHwLanguage, useHwText, type HwTextKey } from '../texts'

interface TopUpFormProps {
  locale?: string
}

/** The preset in the middle of the list; the lowest allowed amount without any. */
function defaultAmount(topUp: HwTopUpLimits): string {
  const preset = topUp.presets[Math.floor(topUp.presets.length / 2)]
  return String(preset ?? topUp.min)
}

/**
 * Picks a list value in whole units of the chosen currency and starts the
 * order. The payable amount and
 * the discount are computed by the server; this form never shows an amount it
 * worked out itself.
 */
export function TopUpForm(props: TopUpFormProps) {
  const { t } = useTranslation()
  const hw = useHwText()
  const language = useHwLanguage()
  const order = useHwTopUpOrder()
  const { status } = useStatus()
  // Only an explicit false closes the form: an older server or a status that
  // is still loading must not hide a payment that works.
  const paymentClosed = status?.hw_payment_available === false
  const { currency, topUp } = useHwCurrency()
  const [amountInput, setAmountInput] = useState(defaultAmount(topUp))
  const [inputCurrency, setInputCurrency] = useState(currency)
  if (inputCurrency !== currency) {
    // Another currency has other limits: start again from its default amount.
    setInputCurrency(currency)
    setAmountInput(defaultAmount(topUp))
  }
  const [errorKey, setErrorKey] = useState<HwTextKey | null>(null)

  const amount = Number(amountInput)
  const amountIsValid =
    amountInput.trim() !== '' &&
    Number.isInteger(amount) &&
    amount >= topUp.min &&
    amount <= topUp.max
  const selectedPreset = topUp.presets.includes(amount) ? amount : null
  const showInvalidAmount = amountInput.trim() !== '' && !amountIsValid

  function changeAmount(next: string) {
    setAmountInput(next)
    setErrorKey(null)
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!amountIsValid) return
    setErrorKey(null)
    order.mutate(
      { amount, currency, language },
      {
        onSuccess: (created) => openCheckout(created.checkout_url),
        onError: (error) => setErrorKey(hwOrderErrorTextKey(error)),
      }
    )
  }

  return (
    <TitledCard
      title={hw('topUpTitle')}
      description={hw('topUpLead')}
      icon={<CreditCard className='h-4 w-4' />}
      iconTone='info'
      disableHoverEffect
    >
      <form onSubmit={submit} className='flex flex-col gap-5'>
        {paymentClosed && (
          <Alert variant='default' role='status'>
            <Hourglass />
            <AlertTitle>{hw('topUpClosedTitle')}</AlertTitle>
            <AlertDescription className='space-y-3'>
              <p>{hw('topUpClosedBody')}</p>
              {/* AlertDescription underlines links; keep this one a button. */}
              <Button
                size='sm'
                className='hover:text-primary-foreground! no-underline!'
                render={
                  <Link
                    to='/dashboard/$section'
                    params={{ section: 'overview' }}
                    hash='startbonus'
                  />
                }
              >
                <Gift className='mr-1 h-3.5 w-3.5' />
                {hw('topUpClosedBonusButton')}
              </Button>
            </AlertDescription>
          </Alert>
        )}

        <div className='flex flex-col gap-2'>
          <span id='hw-topup-preset-label' className='text-sm font-medium'>
            {hw('topUpPresetLabel')}
          </span>
          <div
            role='group'
            aria-labelledby='hw-topup-preset-label'
            className='flex flex-wrap gap-2'
          >
            {topUp.presets.map((preset) => (
              <Button
                key={preset}
                type='button'
                variant={selectedPreset === preset ? 'default' : 'outline'}
                aria-pressed={selectedPreset === preset}
                onClick={() => changeAmount(String(preset))}
              >
                {formatHwAmount(preset, currency, props.locale)}
              </Button>
            ))}
          </div>
        </div>

        <div className='flex flex-col gap-2'>
          <Label htmlFor='hw-topup-amount'>{hw('topUpCustomLabel')}</Label>
          <Input
            id='hw-topup-amount'
            type='number'
            inputMode='numeric'
            min={topUp.min}
            max={topUp.max}
            step={1}
            value={amountInput}
            placeholder={hw('topUpCustomPlaceholder')}
            aria-invalid={showInvalidAmount}
            aria-describedby='hw-topup-amount-note'
            onChange={(event) => changeAmount(event.target.value)}
          />
          <p
            id='hw-topup-amount-note'
            className='text-muted-foreground text-xs'
          >
            {hw('topUpRangeNote', {
              min: formatHwAmount(topUp.min, currency, props.locale),
              max: formatHwAmount(topUp.max, currency, props.locale),
            })}
          </p>
          {showInvalidAmount && (
            <p role='alert' className='text-destructive text-sm'>
              {hw('topUpInvalidAmount')}
            </p>
          )}
        </div>

        <div className='bg-muted/40 flex flex-col gap-1.5 rounded-md p-3 text-sm'>
          {amountIsValid && (
            <p className='font-medium'>
              {hw('topUpValueLabel')}: {formatHwAmount(amount, currency, props.locale)}
            </p>
          )}
          <p>{hw('topUpDiscountRule')}</p>
          <p className='text-muted-foreground text-xs'>
            {[hw('topUpKdvNote'), hw('topUpPayableNote')]
              .filter(Boolean)
              .join(' ')}
          </p>
        </div>

        {errorKey !== null && (
          <p role='alert' className='text-destructive text-sm'>
            {hw(errorKey)}
          </p>
        )}

        <Button
          type='submit'
          disabled={paymentClosed || !amountIsValid || order.isPending}
          className='w-full sm:w-auto'
        >
          {order.isPending ? hw('topUpSubmitting') : hw('topUpSubmit')}
        </Button>
        <p className='text-muted-foreground text-xs'>
          {t('HisarWeb pays the payment and currency conversion costs. You get the full credit for what you pay, with no surcharge or deduction.')}
        </p>
      </form>
    </TitledCard>
  )
}
