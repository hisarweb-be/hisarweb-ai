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
import { Check, Gift, Wallet } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useHwPackages } from '@/features/hw/hooks/use-hw-packages'
import { type HwTextKey, useHwText } from '@/features/hw/texts'
import { useStatus } from '@/hooks/use-status'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

type BonusStep = { key: HwTextKey; done: boolean }

/**
 * Tells a new customer how to receive the start bonus: confirm the e-mail
 * address, link Telegram, share the phone number with the bot. It disappears
 * once the free credit package exists. When the Telegram step is not open on
 * this installation, the card says so instead of sending the customer to a
 * button that cannot work.
 */
export function StartbonusCard() {
  const hw = useHwText()
  const user = useAuthStore((state) => state.auth.user)
  const { status } = useStatus()
  const packages = useHwPackages()

  if (!user || !packages.isSuccess) return null
  const claimed = packages.data.packages.some((pkg) => pkg.source === 'free')
  if (claimed) return null

  const telegramOpen = Boolean(status?.telegram_oauth_configured)
  const botName = String(status?.telegram_bot_name ?? '').replace(/^@/, '')
  const steps: BonusStep[] = [
    { key: 'bonusStepEmail', done: Boolean(user.email) },
    { key: 'bonusStepTelegram', done: Boolean(user.telegram_id) },
    { key: 'bonusStepPhone', done: false },
  ]
  const current = steps.findIndex((step) => !step.done)
  const stepNumber = current + 1

  let action: { label: string; to?: string; href?: string } | null = null
  if (!telegramOpen) {
    action = null
  } else if (current === 0) {
    action = { label: hw('bonusButtonEmail'), to: '/security' }
  } else if (current === 1) {
    action = { label: hw('bonusButtonTelegram'), to: '/security' }
  } else if (botName) {
    action = { label: hw('bonusButtonPhone'), href: `https://t.me/${botName}` }
  }

  return (
    <section
      id='startbonus'
      aria-labelledby='startbonus-title'
      className='bg-card scroll-mt-24 rounded-2xl border p-4 shadow-xs sm:p-5'
    >
      <div className='flex flex-wrap items-start justify-between gap-4'>
        <div className='flex max-w-2xl min-w-0 flex-col gap-1'>
          <div className='text-muted-foreground flex items-center gap-2 text-xs font-medium tracking-wider uppercase'>
            <Gift className='size-3.5' aria-hidden='true' />
            {hw('bonusEyebrow')}
          </div>
          <h3
            id='startbonus-title'
            className='text-xl font-semibold tracking-tight'
          >
            {hw('bonusTitle')}
          </h3>
          <p className='text-muted-foreground text-sm leading-relaxed'>
            {telegramOpen
              ? hw('bonusLead', { step: stepNumber })
              : hw('bonusClosed')}
          </p>
        </div>
        <div className='flex flex-wrap items-center gap-2'>
          {action?.to && (
            <Button size='sm' render={<Link to={action.to} />}>
              <Gift data-icon='inline-start' />
              {action.label}
            </Button>
          )}
          {action?.href && (
            <Button
              size='sm'
              render={<a href={action.href} target='_blank' rel='noreferrer' />}
            >
              <Gift data-icon='inline-start' />
              {action.label}
            </Button>
          )}
          <Button variant='outline' size='sm' render={<Link to='/satin-al' />}>
            <Wallet data-icon='inline-start' />
            {hw('noCreditBuyButton')}
          </Button>
        </div>
      </div>
      <ol className='mt-4 grid gap-2 sm:grid-cols-3'>
        {steps.map((step, index) => (
          <li
            key={step.key}
            className={cn(
              'flex items-start gap-3 rounded-xl border p-3 text-sm',
              index === current && telegramOpen && 'border-primary bg-primary/5'
            )}
          >
            <span
              className={cn(
                'flex size-6 shrink-0 items-center justify-center rounded-full border text-xs font-medium',
                step.done && 'bg-success/10 text-success border-success/30'
              )}
            >
              {step.done ? <Check className='size-3.5' /> : index + 1}
            </span>
            <span className='min-w-0'>{hw(step.key)}</span>
          </li>
        ))}
      </ol>
    </section>
  )
}
