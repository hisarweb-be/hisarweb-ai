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
import { ThumbsDown, ThumbsUp } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'

import { sendAnswerFeedback } from './api'

type AnswerFeedbackProps = {
  question: string
  answer: string
  lang: string
  admin: boolean
}

/** Thumbs under an answer. An admin can add the right answer to a thumbs down. */
export function AnswerFeedback(props: AnswerFeedbackProps) {
  const { t } = useTranslation()
  const [voted, setVoted] = useState(false)
  const [correcting, setCorrecting] = useState(false)
  const [correction, setCorrection] = useState('')

  async function vote(rating: 1 | -1, text?: string) {
    setVoted(true)
    setCorrecting(false)
    try {
      await sendAnswerFeedback(
        {
          question: props.question,
          answer: props.answer,
          rating,
          lang: props.lang,
          correction: text?.trim() || undefined,
        },
        props.admin
      )
    } catch {
      // A lost vote is not worth an error in the chat.
    }
    toast.success(t('Thanks for your feedback'))
  }

  return (
    <div className='mt-2 space-y-2'>
      <div className='flex gap-1'>
        <Button
          type='button'
          size='icon-sm'
          variant='ghost'
          disabled={voted}
          aria-label={t('Helpful answer')}
          onClick={() => void vote(1)}
        >
          <ThumbsUp className='size-3.5' />
        </Button>
        <Button
          type='button'
          size='icon-sm'
          variant='ghost'
          disabled={voted}
          aria-label={t('Not helpful')}
          onClick={() => (props.admin ? setCorrecting(true) : void vote(-1))}
        >
          <ThumbsDown className='size-3.5' />
        </Button>
      </div>
      {correcting && (
        <div className='space-y-2'>
          <Textarea
            value={correction}
            onChange={(event) => setCorrection(event.target.value)}
            placeholder={t('What should the answer have been?')}
            aria-label={t('What should the answer have been?')}
            rows={2}
            maxLength={2000}
          />
          <Button
            type='button'
            size='sm'
            onClick={() => void vote(-1, correction)}
          >
            {t('Save')}
          </Button>
        </div>
      )}
    </div>
  )
}
