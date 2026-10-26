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
import i18next, { type i18n as I18nInstance } from 'i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import { beforeEach, describe, expect, test } from 'vitest'

import {
  INTERFACE_I18N_OPTIONS,
  applyDocumentLanguage,
  lazyLocaleBackend,
} from '../config'
import zhTranslations from '../locales/zh.json'

// The application singleton is initialized by the shared test setup, so the
// language rules are exercised on a fresh instance built from the same options.
// i18next stores the resources object it is given and adds loaded languages to
// it, so every instance gets its own copy.
async function createInterfaceI18n(): Promise<I18nInstance> {
  const instance = i18next.createInstance()
  await instance
    .use(LanguageDetector)
    .use(lazyLocaleBackend)
    .init({
      ...INTERFACE_I18N_OPTIONS,
      resources: { ...INTERFACE_I18N_OPTIONS.resources },
    })
  return instance
}

describe('interface language', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  test('starts in Turkish when the visitor has not chosen a language', async () => {
    const i18n = await createInterfaceI18n()

    expect(i18n.language).toBe('tr')
  })

  test('the stored choice wins over the default language', async () => {
    localStorage.setItem('i18nextLng', 'en')

    const i18n = await createInterfaceI18n()

    expect(i18n.language).toBe('en')
  })

  test('the TR/EN switch changes the language and stores the choice', async () => {
    const i18n = await createInterfaceI18n()

    await i18n.changeLanguage('en')
    expect(i18n.language).toBe('en')

    await i18n.changeLanguage('tr')
    expect(i18n.language).toBe('tr')
    expect(localStorage.getItem('i18nextLng')).toBe('tr')
  })

  test('a key that Turkish does not translate yet falls back to English', async () => {
    const i18n = await createInterfaceI18n()
    i18n.addResource('en', 'translation', '__fallback probe__', 'English only')

    expect(i18n.t('__fallback probe__')).toBe('English only')
  })
})

describe('language files', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  test('only Turkish and English ship in the main bundle', () => {
    // The other six files are about 4 MB together; they load when chosen.
    expect(Object.keys(INTERFACE_I18N_OPTIONS.resources)).toEqual(['tr', 'en'])
  })

  test('a stored other language is loaded before the interface starts in it', async () => {
    localStorage.setItem('i18nextLng', 'zhCN')

    const i18n = await createInterfaceI18n()

    expect(i18n.language).toBe('zhCN')
    expect(i18n.t('About')).toBe(zhTranslations.translation.About)
  })

  test('choosing another language loads its file on demand', async () => {
    const i18n = await createInterfaceI18n()
    expect(i18n.hasResourceBundle('zhCN', 'translation')).toBe(false)

    await i18n.changeLanguage('zhCN')

    expect(i18n.t('About')).toBe(zhTranslations.translation.About)
  })
})

describe('applyDocumentLanguage', () => {
  test('mirrors the interface language onto <html lang>', () => {
    applyDocumentLanguage('tr')
    expect(document.documentElement.lang).toBe('tr')

    applyDocumentLanguage('en')
    expect(document.documentElement.lang).toBe('en')
  })
})
