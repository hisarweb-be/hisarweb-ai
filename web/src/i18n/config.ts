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
import LanguageDetector from 'i18next-browser-languagedetector'
import { initReactI18next } from 'react-i18next'

import { convertDetectedLanguage, toIntlLocale } from './languages'
import en from './locales/en.json'
import fr from './locales/fr.json'
import ja from './locales/ja.json'
import ru from './locales/ru.json'
import tr from './locales/tr.json'
import vi from './locales/vi.json'
import zhTW from './locales/zh-TW.json'
import zhCN from './locales/zh.json'
import { loadTextOverrides } from './text-overrides'

export const resources = {
  tr,
  en,
  zhCN,
  fr,
  ru,
  ja,
  vi,
  zhTW,
} as const

/**
 * Init options of the interface languages, exported so a test can exercise them
 * on its own i18next instance (the application singleton is shared with the
 * whole app and is initialized elsewhere in tests).
 */
export const INTERFACE_I18N_OPTIONS = {
  resources,
  // Turkish is the product language for everyone who has not chosen
  // otherwise, so the browser locale is not consulted (a Turkish visitor
  // with an English browser still gets Turkish). The second entry keeps
  // untranslated keys readable: they resolve to the English source text.
  fallbackLng: ['tr', 'en'],
  supportedLngs: ['tr', 'en', 'zhCN', 'fr', 'ru', 'ja', 'vi', 'zhTW'],
  load: 'currentOnly' as const,
  nsSeparator: false as const, // Allow literal colons in keys (e.g., URLs, labels)
  interpolation: {
    escapeValue: false, // not needed for react as it escapes by default
  },
  detection: {
    // Only an explicit choice is remembered; without one the fallback
    // language (Turkish) applies.
    order: ['localStorage'],
    caches: ['localStorage'],
    // Browsers report `zh-CN`/`zh-TW`/`zh`; map them onto our `zhCN`/`zhTW`
    // codes (non-Chinese codes pass through for normal supportedLngs matching).
    convertDetectedLanguage,
  },
}

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({ ...INTERFACE_I18N_OPTIONS, debug: import.meta.env.DEV })

/** Keep `<html lang>` in step with the interface language (a11y, hyphenation). */
export function applyDocumentLanguage(language?: string | null): void {
  if (typeof document === 'undefined') return
  const locale = toIntlLocale(language)
  if (locale) document.documentElement.lang = locale
}

i18n.on('languageChanged', applyDocumentLanguage)

// Admin-applied text replacements load once per language; loading them
// re-emits languageChanged, which the guard turns into a no-op.
let overridesLoadedFor = ''
function refreshTextOverrides(language?: string | null): void {
  if (!language || language === overridesLoadedFor) return
  if (typeof fetch === 'undefined') return
  overridesLoadedFor = language
  void loadTextOverrides(i18n)
}
i18n.on('languageChanged', refreshTextOverrides)
refreshTextOverrides(i18n.language)
applyDocumentLanguage(i18n.resolvedLanguage ?? i18n.language)

export default i18n
