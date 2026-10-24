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
import { Languages, Check } from 'lucide-react'
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  INTERFACE_LANGUAGE_OPTIONS,
  normalizeInterfaceLanguage,
} from '@/i18n/languages'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

/**
 * The two languages the product is offered in. `tr` is the default; `en` is the
 * fallback for keys that are not translated yet.
 */
const TOGGLE_LANGUAGE_OPTIONS = [
  { code: 'tr', short: 'TR', label: 'Türkçe' },
  { code: 'en', short: 'EN', label: 'English' },
] as const

export interface LanguageSwitcherProps {
  /**
   * - `menu` (default): icon button opening the full language list.
   * - `toggle`: always-visible TR/EN segmented control, used in the headers so
   *   the language choice is one click away on every page.
   */
  variant?: 'menu' | 'toggle'
}

export function LanguageSwitcher(props: LanguageSwitcherProps = {}) {
  const { i18n, t } = useTranslation()
  const user = useAuthStore((s) => s.auth.user)
  const currentLanguage = normalizeInterfaceLanguage(i18n.language)
  const handleChangeLanguage = useCallback(
    async (code: string) => {
      await i18n.changeLanguage(code)
      if (user) {
        try {
          await api.put('/api/user/self', { language: code })
        } catch {
          // Best-effort persistence; don't block the UI on failure
        }
      }
    },
    [i18n, user]
  )

  if (props.variant === 'toggle') {
    return (
      <div
        role='group'
        aria-label={t('Change language')}
        className='border-border/60 inline-flex items-center gap-0.5 rounded-lg border p-0.5'
      >
        {TOGGLE_LANGUAGE_OPTIONS.map((lang) => {
          const isActive = currentLanguage === lang.code
          return (
            <Button
              key={lang.code}
              type='button'
              size='sm'
              variant={isActive ? 'secondary' : 'ghost'}
              aria-pressed={isActive}
              title={lang.label}
              onClick={() => handleChangeLanguage(lang.code)}
              className='h-7 rounded-md px-2 text-xs font-semibold'
            >
              {lang.short}
              <span className='sr-only'> {lang.label}</span>
            </Button>
          )
        })}
      </div>
    )
  }

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        render={<Button variant='ghost' size='icon' className='h-9 w-9' />}
      >
        <Languages className='size-[1.2rem]' />
        <span className='sr-only'>{t('Change language')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end'>
        {INTERFACE_LANGUAGE_OPTIONS.map((lang) => (
          <DropdownMenuItem
            key={lang.code}
            onClick={() => handleChangeLanguage(lang.code)}
          >
            {lang.label}
            <Check
              size={14}
              className={cn(
                'ms-auto',
                currentLanguage !== lang.code && 'hidden'
              )}
            />
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
