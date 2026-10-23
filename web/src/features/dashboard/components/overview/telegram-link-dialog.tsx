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
import { CheckCircle2, Globe, Loader2, Send } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  type TelegramLinks,
  telegramLinks,
} from '@/features/hw/lib/telegram-link'
import { type HwTextKey, useHwText } from '@/features/hw/texts'
import { api } from '@/lib/api'

/** Answer of `POST /api/hw/telegram/link`. */
type TelegramLinkAnswer = {
  success: boolean
  message?: string
  data?: { linked?: boolean; url?: string; expires_at?: number }
}

/** Answer of `GET /api/hw/telegram/link`. */
type TelegramLinkStatus = {
  linked: boolean
  phone_shared: boolean
  bonus_granted: boolean
}

/** How often the dialog asks how far the bot conversation is. */
const POLL_INTERVAL_MS = 4000
/** Polling ends after this; the one-time link itself lives 15 minutes. */
const POLL_LIMIT_MS = 10 * 60 * 1000

interface TelegramLinkDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Called whenever the bot moved the account forward, to read it again. */
  onProgress: () => void | Promise<void>
}

/**
 * Opens the bot chat for the Telegram and phone steps of the start bonus. A
 * `t.me` link alone does nothing on a computer without the Telegram app, so
 * the dialog also shows a QR code for the phone, a Telegram Web button and the
 * app link, and follows the bot conversation until the bonus is granted.
 */
export function TelegramLinkDialog(props: TelegramLinkDialogProps) {
  const hw = useHwText()
  const { t } = useTranslation()
  const { open, onProgress } = props
  const [links, setLinks] = useState<TelegramLinks | null>(null)
  const [loading, setLoading] = useState(false)
  const [startedAt, setStartedAt] = useState(0)
  const [stopped, setStopped] = useState(false)

  const requestLink = useCallback(async () => {
    setLoading(true)
    setLinks(null)
    setStopped(false)
    setStartedAt(Date.now())
    try {
      const answer = (
        await api.post<TelegramLinkAnswer>('/api/hw/telegram/link')
      ).data
      if (!answer.success) {
        toast.error(t(answer.message ?? 'Failed to create the Telegram link'))
        return
      }
      if (answer.data?.url) {
        const derived = telegramLinks(answer.data.url)
        if (derived) setLinks(derived)
        else toast.error(t('Failed to create the Telegram link'))
      }
    } catch (error) {
      const message =
        error instanceof Error && error.message
          ? error.message
          : 'Failed to create the Telegram link'
      toast.error(t(message))
    } finally {
      setLoading(false)
    }
  }, [t])

  // Every opening asks for a fresh one-time link.
  useEffect(() => {
    if (open) void requestLink()
  }, [open, requestLink])

  // Polling ends after ten minutes; the customer can ask for a new link.
  useEffect(() => {
    if (!open || startedAt === 0) return
    const timer = window.setTimeout(
      () => setStopped(true),
      Math.max(0, startedAt + POLL_LIMIT_MS - Date.now())
    )
    return () => window.clearTimeout(timer)
  }, [open, startedAt])

  const status = useQuery({
    queryKey: ['hw', 'telegram-link-status'],
    queryFn: async () => {
      const answer = (
        await api.get<{ success: boolean; data?: TelegramLinkStatus }>(
          '/api/hw/telegram/link'
        )
      ).data
      if (!answer.success || !answer.data) {
        throw new Error('Failed to read the Telegram link status')
      }
      return answer.data
    },
    enabled: open && !stopped,
    refetchInterval: (query) =>
      query.state.data?.bonus_granted ? false : POLL_INTERVAL_MS,
    refetchIntervalInBackground: true,
    gcTime: 0,
  })

  // Each step the bot completes is read back into the account and the card.
  const progress = status.data
    ? `${status.data.linked}-${status.data.phone_shared}-${status.data.bonus_granted}`
    : ''
  useEffect(() => {
    if (open && progress) void onProgress()
  }, [open, progress, onProgress])

  const done = Boolean(status.data?.bonus_granted)
  let stateKey: HwTextKey = 'tgDialogWaiting'
  if (done) stateKey = 'tgDialogDone'
  else if (status.data?.phone_shared) stateKey = 'tgDialogPhoneShared'
  else if (stopped) stateKey = 'tgDialogStopped'
  else if (status.data?.linked) stateKey = 'tgDialogLinked'

  return (
    <Dialog
      open={open}
      onOpenChange={props.onOpenChange}
      title={hw('tgDialogTitle')}
      description={done ? undefined : hw('tgDialogLead')}
      contentClassName='sm:max-w-lg'
      footer={
        <div className='flex w-full flex-wrap justify-end gap-2'>
          {stopped && !done && (
            <Button
              variant='outline'
              onClick={() => void requestLink()}
              disabled={loading}
            >
              {hw('tgDialogRetry')}
            </Button>
          )}
          <Button
            variant={done ? 'default' : 'outline'}
            onClick={() => props.onOpenChange(false)}
          >
            {hw('tgDialogClose')}
          </Button>
        </div>
      }
    >
      <div className='flex flex-col gap-4'>
        <Alert role='status' aria-live='polite'>
          {done ? (
            <CheckCircle2 className='text-success' />
          ) : (
            <Loader2 className={stopped ? undefined : 'animate-spin'} />
          )}
          <AlertDescription
            className={done ? 'text-success font-medium' : undefined}
          >
            {hw(stateKey)}
          </AlertDescription>
        </Alert>

        {!done && loading && (
          <p className='text-muted-foreground text-sm'>
            {hw('tgDialogLoading')}
          </p>
        )}

        {!done && links && (
          <>
            <div className='flex flex-col items-center gap-2'>
              <div className='rounded-lg bg-white p-3'>
                <QRCodeSVG
                  value={links.web}
                  size={176}
                  title={hw('tgDialogQrLabel')}
                />
              </div>
              <span className='text-muted-foreground text-xs'>
                {hw('tgDialogQrLabel')}
              </span>
            </div>
            <div className='grid gap-2 sm:grid-cols-2'>
              <Button
                render={
                  <a href={links.browser} target='_blank' rel='noreferrer' />
                }
              >
                <Globe data-icon='inline-start' />
                {hw('tgDialogOpenWeb')}
              </Button>
              <Button variant='outline' render={<a href={links.app} />}>
                <Send data-icon='inline-start' />
                {hw('tgDialogOpenApp')}
              </Button>
            </div>
            <p className='text-muted-foreground text-xs leading-relaxed'>
              {hw('tgDialogNoAppHint')}
            </p>
            <div className='bg-muted flex items-center gap-2 rounded-lg p-3'>
              <div className='flex min-w-0 flex-1 flex-col gap-1'>
                <span className='text-muted-foreground text-xs'>
                  {hw('tgDialogFallback')}
                </span>
                <a
                  href={links.web}
                  target='_blank'
                  rel='noreferrer'
                  className='truncate font-mono text-xs underline-offset-2 hover:underline'
                >
                  {links.web}
                </a>
              </div>
              <CopyButton value={links.web} variant='ghost' size='icon' />
            </div>
            <p className='text-muted-foreground text-xs'>
              {hw('tgDialogValidity')}
            </p>
          </>
        )}
      </div>
    </Dialog>
  )
}
