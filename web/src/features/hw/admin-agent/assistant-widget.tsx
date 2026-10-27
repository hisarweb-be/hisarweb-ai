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
import { Paperclip, Send, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
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

import { AnswerFeedback } from './answer-feedback'
import {
  askAdminAgent,
  askAssistantChat,
  askAssistantChip,
  assistantLanguage,
  escalateToBureau,
  fetchAssistantFaq,
  revertTextOverride,
  type AgentAnswer,
  type AgentAttachment,
  type AgentTextFix,
  type AgentTurn,
  type AssistantFaq,
} from './api'
import { applyFix, type AppliedFix } from './apply-fix'
import {
  filesFromClipboard,
  MAX_ATTACHMENTS,
  readAttachment,
} from './attachments'
import { TelegramLogo } from './telegram-logo'

type Entry = AgentTurn & {
  id: number
  answer?: AgentAnswer
  applied?: AppliedFix
  /** A real answer (model or chip), so it can be rated. */
  rateable?: boolean
}

const fileKey = (file: AgentAttachment) =>
  file.name + file.data.length + file.data.slice(-24)

function messageOf(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

/**
 * The chat on every page. A clicked question shows its fixed answer; typed
 * text always goes to the model. An admin can also report an error, a hard
 * word or a screenshot, and the assistant proposes a text fix or a bureau card.
 */
export function AssistantWidget() {
  const isAdmin = useIsAdmin()
  const { t, i18n } = useTranslation()
  const lang = assistantLanguage(i18n?.language)
  const [open, setOpen] = useState(false)
  const [entries, setEntries] = useState<Entry[]>([])
  const [draft, setDraft] = useState('')
  const [pending, setPending] = useState<AgentAttachment[]>([])
  const [faq, setFaq] = useState<AssistantFaq[]>([])
  const [busy, setBusy] = useState(false)
  const lastId = useRef(0)
  const fileInput = useRef<HTMLInputElement>(null)
  const bottom = useRef<HTMLDivElement>(null)

  // Like Telegram: the newest message and the typing bubble stay in view.
  useEffect(() => {
    bottom.current?.scrollIntoView?.({ block: 'end', behavior: 'smooth' })
  }, [entries, busy, open])

  useEffect(() => {
    if (!open) return
    fetchAssistantFaq(lang)
      .then(setFaq)
      .catch(() => setFaq([]))
  }, [open, lang])

  const failed = t('Request failed')
  const patch = (index: number, change: Partial<Entry>) =>
    setEntries((current) =>
      current.map((entry, at) =>
        at === index ? { ...entry, ...change } : entry
      )
    )

  async function addFiles(files: File[]) {
    let count = pending.length
    for (const file of files) {
      if (count >= MAX_ATTACHMENTS) {
        toast.error(t('At most 3 attachments per message.'))
        return
      }
      const result = await readAttachment(file)
      if (typeof result === 'string') {
        toast.error(
          result === 'size'
            ? t('This file is too large.')
            : t('Only images and text files can be attached.')
        )
      } else {
        count += 1
        setPending((current) => [...current, result])
      }
    }
  }

  function begin(text: string, attachments: AgentAttachment[]): Entry[] {
    const history: Entry[] = [
      ...entries,
      {
        id: ++lastId.current,
        role: 'user',
        content: text,
        attachments: attachments.length ? attachments : undefined,
      },
    ]
    setEntries(history)
    setDraft('')
    setPending([])
    setBusy(true)
    return history
  }

  async function pickChip(item: AssistantFaq) {
    if (busy) return
    const history = begin(item.question, [])
    const counted = askAssistantChip(item.id, lang, isAdmin).catch(
      () => undefined
    )
    const result = isAdmin ? await counted : undefined
    setEntries([
      ...history,
      {
        id: ++lastId.current,
        role: 'assistant',
        content: result?.matched ? result.answer : item.answer,
        rateable: true,
      },
    ])
    setBusy(false)
  }

  async function send() {
    const text = draft.trim()
    if ((!text && pending.length === 0) || busy) return
    const history = begin(text, pending)
    const reply = (entry: Partial<Entry> & { content: string }) =>
      setEntries([
        ...history,
        { id: ++lastId.current, role: 'assistant', ...entry },
      ])
    try {
      if (isAdmin) {
        const answer = await askAdminAgent(
          history.map(({ role, content, attachments: files }, at) => ({
            role,
            content,
            attachments: at === history.length - 1 ? files : undefined,
          })),
          lang
        )
        reply({ content: answer.reply, answer, rateable: true })
      } else {
        reply({
          content: await askAssistantChat(history, lang),
          rateable: true,
        })
      }
    } catch {
      reply({
        content: t('The answer is delayed. Please try again in a minute.'),
      })
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

  const title = isAdmin ? t('Admin assistant') : t('Assistant')
  const placeholder = isAdmin
    ? t('What is wrong or unclear?')
    : t('Type your question')

  return (
    <>
      <button
        type='button'
        className='fixed right-4 bottom-4 z-40 size-14 rounded-full shadow-lg transition-transform hover:scale-105 focus-visible:ring-2 focus-visible:ring-offset-2'
        aria-label={title}
        onClick={() => setOpen(true)}
      >
        <TelegramLogo className='size-full' />
      </button>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent className='flex w-full flex-col gap-3 sm:max-w-md'>
          <SheetHeader>
            <SheetTitle>{title}</SheetTitle>
            <SheetDescription>
              {isAdmin
                ? t(
                    'Report an error or a hard word. The assistant suggests a fix.'
                  )
                : t('Pick a question or type your own.')}
            </SheetDescription>
          </SheetHeader>
          <div className='flex-1 space-y-3 overflow-y-auto px-4'>
            <div className='flex flex-wrap gap-2'>
              {faq.map((item) => (
                <Button
                  key={item.id}
                  type='button'
                  size='sm'
                  variant='outline'
                  disabled={busy}
                  onClick={() => void pickChip(item)}
                >
                  {item.question}
                </Button>
              ))}
            </div>
            {entries.map((entry, index) => (
              <div
                key={entry.id}
                className={
                  entry.role === 'user'
                    ? 'bg-primary text-primary-foreground ml-8 rounded-lg p-3 text-sm'
                    : 'bg-muted mr-8 rounded-lg p-3 text-sm'
                }
              >
                {entry.attachments?.map((file) =>
                  file.mime.startsWith('image/') ? (
                    <img
                      key={fileKey(file)}
                      src={file.data}
                      alt={file.name}
                      className='mb-2 max-h-48 rounded-md'
                    />
                  ) : (
                    <p key={fileKey(file)} className='mb-1 text-xs opacity-80'>
                      {file.name}
                    </p>
                  )
                )}
                <p className='whitespace-pre-wrap'>{entry.content}</p>
                {entry.rateable && (
                  <AnswerFeedback
                    question={entries[index - 1]?.content ?? ''}
                    answer={entry.content}
                    lang={lang}
                    admin={isAdmin}
                  />
                )}
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
            {busy && (
              <div
                role='status'
                className='bg-muted text-muted-foreground mr-8 flex w-fit items-center gap-1 rounded-lg p-3 text-sm'
              >
                {t('typing')}
                <span aria-hidden className='flex gap-0.5'>
                  {[0, 150, 300].map((delay) => (
                    <span
                      key={delay}
                      className='size-1 animate-bounce rounded-full bg-current'
                      style={{ animationDelay: `${delay}ms` }}
                    />
                  ))}
                </span>
              </div>
            )}
            <div ref={bottom} />
          </div>
          {pending.length > 0 && (
            <div className='flex flex-wrap gap-2 px-4'>
              {pending.map((file, at) => (
                <span
                  key={fileKey(file)}
                  className='bg-muted flex items-center gap-1 rounded-md p-1 text-xs'
                >
                  {file.mime.startsWith('image/') ? (
                    <img
                      src={file.data}
                      alt={file.name}
                      className='size-10 rounded object-cover'
                    />
                  ) : (
                    file.name
                  )}
                  <button
                    type='button'
                    aria-label={t('Remove attachment')}
                    onClick={() =>
                      setPending((current) =>
                        current.filter((_, index) => index !== at)
                      )
                    }
                  >
                    <X className='size-3' />
                  </button>
                </span>
              ))}
            </div>
          )}
          <form
            className='flex items-end gap-2 px-4 pb-4'
            onSubmit={(event) => {
              event.preventDefault()
              void send()
            }}
            onDragOver={(event) => isAdmin && event.preventDefault()}
            onDrop={(event) => {
              if (!isAdmin) return
              event.preventDefault()
              void addFiles([...event.dataTransfer.files])
            }}
          >
            {isAdmin && (
              <>
                <input
                  ref={fileInput}
                  type='file'
                  hidden
                  multiple
                  accept='image/*,text/*,.log,.json,.csv,.md'
                  onChange={(event) => {
                    void addFiles([...(event.target.files ?? [])])
                    event.target.value = ''
                  }}
                />
                <Button
                  type='button'
                  size='icon'
                  variant='outline'
                  aria-label={t('Attach a file')}
                  onClick={() => fileInput.current?.click()}
                >
                  <Paperclip className='size-4' />
                </Button>
              </>
            )}
            <Textarea
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={(event) => {
                // Enter sends, Shift+Enter starts a new line.
                if (
                  event.key !== 'Enter' ||
                  event.shiftKey ||
                  event.nativeEvent.isComposing
                ) {
                  return
                }
                event.preventDefault()
                void send()
              }}
              enterKeyHint='send'
              onPaste={(event) => {
                const files = filesFromClipboard(event.clipboardData)
                if (isAdmin && files.length > 0) {
                  event.preventDefault()
                  void addFiles(files)
                }
              }}
              placeholder={placeholder}
              rows={2}
              maxLength={2000}
              aria-label={placeholder}
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
