import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'

// Step-up before a relay key is shown in full (OWASP ASVS V2.10.4, gap G2).
// The server wants the account password, or a 2FA code when 2FA is on.

export type HwKeyResponse = {
  success: boolean
  message?: string
  code?: string
  data?: unknown
}

const STEP_UP_CODES = new Set([
  'PASSWORD_REQUIRED',
  'PASSWORD_INVALID',
  'TWO_FA_REQUIRED',
  'TWO_FA_INVALID',
])

function responseOf(error: unknown): HwKeyResponse | undefined {
  return (error as { response?: { data?: HwKeyResponse } })?.response?.data
}

export function KeyStepUpDialog(props: {
  url: string
  body: Record<string, unknown>
  onDone: (result: HwKeyResponse) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(true)
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [needsCode, setNeedsCode] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const finish = (result: HwKeyResponse) => {
    setOpen(false)
    props.onDone(result)
  }

  async function submit() {
    setBusy(true)
    setError('')
    let result: HwKeyResponse | undefined
    try {
      const res = await api.post(
        props.url,
        {
          ...props.body,
          ...(password ? { password } : {}),
          ...(code ? { two_fa_code: code } : {}),
        },
        { skipBusinessError: true, skipErrorHandler: true },
      )
      result = res.data as HwKeyResponse
    } catch (e) {
      result = responseOf(e)
      result ??= { success: false, message: (e as Error).message }
    } finally {
      setBusy(false)
    }
    if (result.success || !STEP_UP_CODES.has(result.code ?? '')) {
      finish(result)
      return
    }
    if (result.code?.startsWith('TWO_FA')) {
      setNeedsCode(true)
    }
    setCode('')
    if (result.code === 'PASSWORD_INVALID') {
      setError(t('Incorrect password'))
    } else if (result.code === 'TWO_FA_INVALID') {
      setError(t('Invalid two-factor authentication code'))
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !busy) {
          finish({ success: false, message: t('Verification cancelled') })
        }
      }}
    >
      <DialogContent>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            void submit()
          }}
        >
          <DialogHeader>
            <DialogTitle>{t('Confirm it is you')}</DialogTitle>
            <DialogDescription>
              {needsCode
                ? t('Enter your two-factor authentication code to show the key')
                : t('Enter your password to show the key')}
            </DialogDescription>
          </DialogHeader>
          {needsCode ? (
            <div className="grid gap-2">
              <Label htmlFor="hw-key-code">{t('Authentication code')}</Label>
              <Input
                id="hw-key-code"
                autoFocus
                autoComplete="one-time-code"
                inputMode="numeric"
                value={code}
                onChange={(e) => setCode(e.target.value)}
              />
            </div>
          ) : (
            <div className="grid gap-2">
              <Label htmlFor="hw-key-password">{t('Password')}</Label>
              <Input
                id="hw-key-password"
                type="password"
                autoFocus
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
          )}
          {error && (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button
              type="submit"
              disabled={busy || !(needsCode ? code : password)}
            >
              {t('Show key')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
