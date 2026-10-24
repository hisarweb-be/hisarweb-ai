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
import { Bot, Send } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import { useIsAdmin } from '@/hooks/use-admin'

import {
  askAdminAgent,
  escalateToBureau,
  revertTextOverride,
  type AgentAnswer,
  type AgentTextFix,
  type AgentTurn,
} from './api'
import { applyFix, type AppliedFix } from './apply-fix'

type Entry = AgentTurn & {
  id: number
  answer?: AgentAnswer
  applied?: AppliedFix
}

function messageOf(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

export function AdminAgentPanel() {
  const isAdmin = useIsAdmin()
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const [entries, setEntries] = useState<Entry[]>([])
  const [draft, setDraft] = useState('')
  const [busy, setBusy] = useState(false)
  const lastId = useRef(0)

  if (!isAdmin) return null

  const failed = t('Request failed')
  const patch = (index: number, change: Partial<Entry>) =>
    setEntries((current) =>
      current.map((entry, at) =>
        at === index ? { ...entry, ...change } : entry
      )
    )

  async function send() {
    const text = draft.trim()
    if (!text || busy) return
    const history: Entry[] = [
      ...entries,
      { id: ++lastId.current, role: 'user', content: text },
    ]
    setEntries(history)
    setDraft('')
    setBusy(true)
    try {
      const answer = await askAdminAgent(
        history.map(({ role, content }) => ({ role, content }))
      )
      setEntries([
        ...history,
        {
          id: ++lastId.current,
          role: 'assistant',
          content: answer.reply,
          answer,
        },
      ])
    } catch (error) {
      toast.error(messageOf(error, failed))
    } finally {
      setBusy(false)
    }
  }

  async function apply(index: number, fix: AgentTextFix) {
    try {
      const result = await applyFix(i18n, fix)
      if (result === 'not-found') {
        toast.error(t('This text was not found on the screen.'))
      } else if (result === 'too-many') {
        toast.error(
          t('This text is used in too many places to change at once.')
        )
      } else {
        patch(index, { applied: result })
        toast.success(t('Text changed.'))
      }
    } catch (error) {
      toast.error(messageOf(error, failed))
    }
  }

  async function undo(index: number, applied: AppliedFix) {
    try {
      for (const key of applied.keys) {
        await revertTextOverride(applied.lang, key)
      }
      patch(index, { applied: undefined })
      toast.success(t('The original text is back after a reload.'))
    } catch (error) {
      toast.error(messageOf(error, failed))
    }
  }

  async function escalate(answer: AgentAnswer) {
    if (!answer.escalate) return
    try {
      const card = await escalateToBureau(
        answer.escalate,
        window.location.pathname
      )
      toast.success(t('Card created: {{card}}', { card }))
    } catch (error) {
      toast.error(messageOf(error, failed))
    }
  }

  return (
    <>
      <Button
        type='button'
        size='icon'
        className='fixed right-4 bottom-4 z-40 rounded-full shadow-lg'
        aria-label={t('Admin assistant')}
        onClick={() => setOpen(true)}
      >
        <Bot className='size-5' />
      </Button>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent className='flex w-full flex-col gap-3 sm:max-w-md'>
          <SheetHeader>
            <SheetTitle>{t('Admin assistant')}</SheetTitle>
            <SheetDescription>
              {t('Report an error or a hard word. The assistant suggests a fix.')}
            </SheetDescription>
          </SheetHeader>
          <div className='flex-1 space-y-3 overflow-y-auto px-4'>
            {entries.map((entry, index) => (
              <div
                key={entry.id}
                className={
                  entry.role === 'user'
                    ? 'bg-primary text-primary-foreground ml-8 rounded-lg p-3 text-sm'
                    : 'bg-muted mr-8 rounded-lg p-3 text-sm'
                }
              >
                <p className='whitespace-pre-wrap'>{entry.content}</p>
                {entry.answer?.text_fix && (
                  <div className='mt-2 space-y-2 border-t pt-2'>
                    <p>
                      <span className='line-through opacity-70'>
                        {entry.answer.text_fix.shown}
                      </span>
                      {' → '}
                      <strong>{entry.answer.text_fix.proposed}</strong>
                    </p>
                    {entry.applied ? (
                      <Button
                        size='sm'
                        variant='outline'
                        onClick={() => undo(index, entry.applied as AppliedFix)}
                      >
                        {t('Undo')}
                      </Button>
                    ) : (
                      <Button
                        size='sm'
                        onClick={() =>
                          apply(index, entry.answer?.text_fix as AgentTextFix)
                        }
                      >
                        {t('Apply this text')}
                      </Button>
                    )}
                  </div>
                )}
                {entry.answer?.escalate && (
                  <div className='mt-2 border-t pt-2'>
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => escalate(entry.answer as AgentAnswer)}
                    >
                      {t('Send to the bureau')}
                    </Button>
                  </div>
                )}
              </div>
            ))}
          </div>
          <form
            className='flex items-end gap-2 px-4 pb-4'
            onSubmit={(event) => {
              event.preventDefault()
              void send()
            }}
          >
            <Textarea
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              placeholder={t('What is wrong or unclear?')}
              rows={2}
              maxLength={2000}
              aria-label={t('What is wrong or unclear?')}
            />
            <Button
              type='submit'
              size='icon'
              disabled={busy}
              aria-label={t('Send')}
            >
              <Send className='size-4' />
            </Button>
          </form>
        </SheetContent>
      </Sheet>
    </>
  )
}
