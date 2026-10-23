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
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Check, Gift, Wallet } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  hwPackagesQueryKey,
  useHwPackages,
} from '@/features/hw/hooks/use-hw-packages'
import { type HwTextKey, useHwText } from '@/features/hw/texts'
import { useStatus } from '@/hooks/use-status'
import { getSelf } from '@/lib/api'
import { cn } from '@/lib/utils'
import { type AuthUser, useAuthStore } from '@/stores/auth-store'

import { TelegramLinkDialog } from './telegram-link-dialog'

type BonusStepAction = {
  label: string
  hint?: HwTextKey
  to?: string
  href?: string
  /** Open the dialog with the one-time bot link instead of navigating. */
  linkTelegram?: boolean
}

type BonusStep = {
  key: HwTextKey
  done: boolean
  /** Telegram and the phone step wait until verification opens. */
  closed: boolean
  /** What the customer does next; empty while the step is closed. */
  action?: BonusStepAction
}

/**
 * Tells a new customer how to receive the start bonus: confirm the e-mail
 * address, link Telegram, share the phone number with the bot. It disappears
 * once the free credit package exists. When the Telegram step is not open on
 * this installation, the card says so instead of sending the customer to a
 * button that cannot work.
 */
export function StartbonusCard() {
  const hw = useHwText()
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const setUser = useAuthStore((state) => state.auth.setUser)
  const { status } = useStatus()
  const packages = useHwPackages()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)

  /** Read the account and the packages again, without polling. */
  const refreshAccount = useCallback(async () => {
    try {
      const self = await getSelf()
      if (self.success && self.data) setUser(self.data as AuthUser)
    } catch {
      // Keep what is on screen; the next return to this window tries again.
    }
    await queryClient.refetchQueries({ queryKey: hwPackagesQueryKey })
  }, [queryClient, setUser])

  // Coming back to this window is the moment the customer may have finished
  // the step in Telegram, so the account is read once more — no timer.
  useEffect(() => {
    const onFocus = () => {
      void refreshAccount()
    }
    window.addEventListener('focus', onFocus)
    return () => window.removeEventListener('focus', onFocus)
  }, [refreshAccount])

  const linkTelegram = () => setDialogOpen(true)
  const dialog = (
    <TelegramLinkDialog
      open={dialogOpen}
      onOpenChange={setDialogOpen}
      onProgress={refreshAccount}
    />
  )

  if (!user || !packages.isSuccess) return null
  const claimed = packages.data.packages.some((pkg) => pkg.source === 'free')
  // The dialog stays on screen to say the bonus arrived, even though the card
  // itself is no longer needed then.
  if (claimed) return dialogOpen ? dialog : null

  const botName = String(status?.telegram_bot_name ?? '').replace(/^@/, '')
  const telegramOpen =
    Boolean(botName) || Boolean(status?.telegram_oauth_configured)
  const steps: BonusStep[] = [
    {
      key: 'bonusStepEmail',
      done: Boolean(user.email),
      closed: false,
      action: {
        label: hw('bonusButtonEmail'),
        hint: 'bonusHintEmail',
        to: '/security',
      },
    },
    {
      key: 'bonusStepTelegram',
      done: Boolean(user.telegram_id),
      closed: !telegramOpen,
      action: telegramOpen
        ? {
            label: t('Link Telegram'),
            hint: 'bonusHintTelegram',
            linkTelegram: true,
          }
        : undefined,
    },
    {
      key: 'bonusStepPhone',
      done: Boolean(packages.data.phone_shared),
      closed: !telegramOpen,
      action:
        telegramOpen && botName
          ? {
              label: hw('bonusButtonPhone'),
              hint: 'bonusHintPhone',
              linkTelegram: true,
            }
          : undefined,
    },
  ]
  const current = steps.findIndex((step) => !step.done)
  const stepNumber = current + 1

  // The e-mail step can always be done, so its call to action stays visible
  // while Telegram verification is closed; the other two cannot work then.
  let action: BonusStepAction | null = null
  if (current === 0) {
    action = { label: hw('bonusButtonEmail'), to: '/security' }
  } else if (telegramOpen && current === 1) {
    action = { label: t('Link Telegram'), linkTelegram: true }
  } else if (telegramOpen && current === 2 && botName) {
    action = { label: hw('bonusButtonPhone'), linkTelegram: true }
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
          {action?.linkTelegram && (
            <Button size='sm' onClick={linkTelegram}>
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
        {steps.map((step, index) => {
          let statusKey: HwTextKey = 'bonusStatusTodo'
          if (step.done) {
            statusKey = 'bonusStatusDone'
          } else if (step.closed) {
            statusKey = 'bonusStatusClosed'
          }
          const stepAction = step.done ? undefined : step.action
          return (
            <li
              key={step.key}
              className={cn(
                'flex items-start gap-3 rounded-xl border p-3 text-sm',
                index === current &&
                  telegramOpen &&
                  'border-primary bg-primary/5'
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
              <div className='flex min-w-0 flex-col gap-1'>
                <span>{hw(step.key)}</span>
                <span
                  className={cn(
                    'text-muted-foreground text-xs',
                    step.done && 'text-success'
                  )}
                >
                  {hw(statusKey)}
                </span>
                {stepAction?.hint && (
                  <span className='text-muted-foreground text-xs leading-relaxed'>
                    {hw(stepAction.hint)}
                  </span>
                )}
                {stepAction?.to && (
                  <Button
                    variant='link'
                    size='sm'
                    className='self-start'
                    render={<Link to={stepAction.to} />}
                  >
                    {stepAction.label}
                  </Button>
                )}
                {stepAction?.href && (
                  <Button
                    variant='link'
                    size='sm'
                    className='self-start'
                    render={
                      <a
                        href={stepAction.href}
                        target='_blank'
                        rel='noreferrer'
                      />
                    }
                  >
                    {stepAction.label}
                  </Button>
                )}
                {stepAction?.linkTelegram && (
                  <Button
                    variant='link'
                    size='sm'
                    className='self-start'
                    onClick={linkTelegram}
                  >
                    {stepAction.label}
                  </Button>
                )}
              </div>
            </li>
          )
        })}
      </ol>
      {dialog}
    </section>
  )
}
