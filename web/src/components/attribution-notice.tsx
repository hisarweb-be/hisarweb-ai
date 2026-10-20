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
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

/**
 * The licence notice AGPL-3.0 section 7 requires us to keep visible: one line
 * that names the upstream project and its licence with a link to the original
 * source, the upstream NOTICE line quoted verbatim, and a link to our own
 * source. This is the only place the upstream project's name is shown; no other
 * screen mentions it.
 */
export function AttributionNotice(props: { className?: string }) {
  const { t } = useTranslation()

  return (
    <span className={cn('text-muted-foreground/45', props.className)}>
      <a
        href='https://github.com/hisarweb-be/hisarweb-ai'
        target='_blank'
        rel='noopener noreferrer'
        className='text-foreground/70 hover:text-foreground font-medium transition-colors'
      >
        {t('Source code')}
      </a>
      <br />
      <a
        href='https://github.com/QuantumNous/new-api'
        target='_blank'
        rel='noopener noreferrer'
        className='hover:text-foreground transition-colors'
      >
        {t('HisarWeb AI is based on New API (AGPL-3.0) by QuantumNous.')}
      </a>
      <br />
      {
        // Required NOTICE line of the upstream project, kept in its original
        // wording in every language, so it is deliberately not translated.
        'Frontend design and development by New API contributors.'
      }
    </span>
  )
}
