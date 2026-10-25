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
import { cn } from '@/lib/utils'

/**
 * The Telegram logo (blue disc with the white paper plane) with a green
 * "online" dot, the icon of the chat on every page.
 */
export function TelegramLogo({ className }: { className?: string }) {
  return (
    <span className={cn('relative inline-flex', className)}>
      <svg viewBox='0 0 240 240' className='size-full' aria-hidden='true'>
        <defs>
          <linearGradient
            id='tg-logo-a'
            x1='0.667'
            y1='0.167'
            x2='0.417'
            y2='0.75'
          >
            <stop offset='0' stopColor='#37aee2' />
            <stop offset='1' stopColor='#1e96c8' />
          </linearGradient>
          <linearGradient
            id='tg-logo-b'
            x1='0.66'
            y1='0.437'
            x2='0.85'
            y2='0.802'
          >
            <stop offset='0' stopColor='#eff7fc' />
            <stop offset='1' stopColor='#fff' />
          </linearGradient>
        </defs>
        <circle cx='120' cy='120' r='120' fill='url(#tg-logo-a)' />
        <path
          fill='#c8daea'
          d='M98 175c-3.888 0-3.227-1.468-4.568-5.17L82 132.207 170 80'
        />
        <path
          fill='#a9c9dd'
          d='M98 175c3 0 4.325-1.372 6-3l16-15.558-19.958-12.035'
        />
        <path
          fill='url(#tg-logo-b)'
          d='M100.04 144.41l48.36 35.729c5.519 3.045 9.501 1.468 10.876-5.123l19.685-92.763c2.015-8.08-3.08-11.746-8.36-9.349l-115.59 44.571c-7.89 3.165-7.843 7.567-1.438 9.528l29.663 9.259 68.673-43.325c3.242-1.966 6.218-.91 3.776 1.258'
        />
      </svg>
      <span
        data-testid='assistant-online'
        className='absolute -right-0.5 -bottom-0.5 size-3.5 rounded-full border-2 border-white bg-green-500'
        title='online'
      />
    </span>
  )
}
