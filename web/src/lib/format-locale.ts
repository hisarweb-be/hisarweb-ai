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
import i18n from 'i18next'

import { toIntlLocale } from '@/i18n/languages'

/** Notation of the product: Turkish (1.234,56, dd.MM.yyyy, 24-hour clock). */
export const APP_LOCALE = 'tr-TR'

/** Every date and time of the customer interface is shown in Istanbul time. */
export const APP_TIME_ZONE = 'Europe/Istanbul'

/**
 * Intl locale for numbers and money when the caller passes none: the active
 * interface language, and Turkish before a language is known. It never follows
 * the browser locale, so a customer sees the same notation on every machine.
 */
export function getFormatLocale(): string {
  return toIntlLocale(i18n.resolvedLanguage || i18n.language) ?? APP_LOCALE
}
