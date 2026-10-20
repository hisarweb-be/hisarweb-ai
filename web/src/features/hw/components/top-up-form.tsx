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
import { CreditCard } from 'lucide-react'
import { useState, type FormEvent } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { TitledCard } from '@/components/ui/titled-card'

import { HW_TOPUP_MAX_TRY, HW_TOPUP_MIN_TRY, HW_TOPUP_PRESET_TRY } from '../api'
import { useHwTopUpOrder } from '../hooks/use-hw-top-up-order'
import { hwOrderErrorTextKey, openCheckout } from '../lib/checkout'
import { formatHwTry } from '../lib/format'
import { useHwLanguage, useHwText, type HwTextKey } from '../texts'

interface TopUpFormProps {
  locale?: string
}

/**
 * Picks a list value in whole lira and starts the order. The payable amount and
 * the discount are computed by the server; this form never shows an amount it
 * worked out itself.
 */
export function TopUpForm(props: TopUpFormProps) {
  const hw = useHwText()
  const language = useHwLanguage()
  const order = useHwTopUpOrder()
  const [amountInput, setAmountInput] = useState('1000')
  const [errorKey, setErrorKey] = useState<HwTextKey | null>(null)

  const amount = Number(amountInput)
  const amountIsValid =
    amountInput.trim() !== '' &&
    Number.isInteger(amount) &&
    amount >= HW_TOPUP_MIN_TRY &&
    amount <= HW_TOPUP_MAX_TRY
  const selectedPreset = HW_TOPUP_PRESET_TRY.includes(amount)
    ? amount
    : null
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
      { package_try: amount, language },
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
        <div className='flex flex-col gap-2'>
          <span id='hw-topup-preset-label' className='text-sm font-medium'>
            {hw('topUpPresetLabel')}
          </span>
          <div
            role='group'
            aria-labelledby='hw-topup-preset-label'
            className='flex flex-wrap gap-2'
          >
            {HW_TOPUP_PRESET_TRY.map((preset) => (
              <Button
                key={preset}
                type='button'
                variant={selectedPreset === preset ? 'default' : 'outline'}
                aria-pressed={selectedPreset === preset}
                onClick={() => changeAmount(String(preset))}
              >
                {formatHwTry(preset, props.locale)}
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
            min={HW_TOPUP_MIN_TRY}
            max={HW_TOPUP_MAX_TRY}
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
              min: formatHwTry(HW_TOPUP_MIN_TRY, props.locale),
              max: formatHwTry(HW_TOPUP_MAX_TRY, props.locale),
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
              {hw('topUpValueLabel')}: {formatHwTry(amount, props.locale)}
            </p>
          )}
          <p>{hw('topUpDiscountRule')}</p>
          <p className='text-muted-foreground text-xs'>
            {hw('topUpKdvNote')} {hw('topUpPayableNote')}
          </p>
        </div>

        {errorKey !== null && (
          <p role='alert' className='text-destructive text-sm'>
            {hw(errorKey)}
          </p>
        )}

        <Button
          type='submit'
          disabled={!amountIsValid || order.isPending}
          className='w-full sm:w-auto'
        >
          {order.isPending ? hw('topUpSubmitting') : hw('topUpSubmit')}
        </Button>
      </form>
    </TitledCard>
  )
}
