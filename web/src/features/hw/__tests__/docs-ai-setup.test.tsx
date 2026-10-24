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
import { render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { AI_SETUP_ANCHOR } from '../components/docs-ai-setup'
import { DocsPage } from '../components/docs-page'
import { AI_SETUP_PROMPTS } from '../docs-ai-setup-prompts'
import { aiSetupTexts } from '../docs-ai-setup-texts'
import { KEY_PLACEHOLDER } from '../docs-tool-guides'

vi.mock('@/components/layout', () => ({
  PublicLayout: (props: { children?: React.ReactNode }) => (
    <main data-testid='public-layout'>{props.children}</main>
  ),
}))
vi.mock('@/components/layout/components/footer', () => ({
  Footer: () => <footer data-testid='site-footer' />,
}))

const trI18n = createInstance()
await trI18n.init({
  lng: 'tr',
  resources: { tr: { translation: {} } },
  initAsync: false,
})

function renderDocs() {
  return render(
    <I18nextProvider i18n={trI18n}>
      <DocsPage />
    </I18nextProvider>
  )
}

afterEach(() => {
  window.history.replaceState(null, '', '/')
  vi.restoreAllMocks()
})

describe('docs page, let the AI do the setup', () => {
  it('has the anchor the vibe coding page links to', () => {
    renderDocs()

    const section = document.querySelector(`#${AI_SETUP_ANCHOR}`)
    expect(AI_SETUP_ANCHOR).toBe('kurulumu-yapay-zekaya-yaptirin')
    expect(section).not.toBeNull()
    expect(section).toHaveTextContent(
      'Kod bilmiyor musunuz? Kurulumu yapay zekaya yaptırın'
    )
    expect(section).toHaveTextContent('Bu modun riski')
    expect(
      screen.getByRole('figure', { name: /dört adım/ })
    ).toBeInTheDocument()
  })

  it('links back to the key step on the same page', () => {
    renderDocs()

    const link = screen.getByRole('link', {
      name: '3. adıma dön: API anahtarı oluşturun',
    })
    expect(link).toHaveAttribute('href', '#adim-3')
    expect(document.querySelector('#adim-3')).toHaveTextContent(
      'API anahtarı oluşturun'
    )
  })

  it('shows both prompts with the key placeholder', () => {
    renderDocs()

    // The code block renders each line on its own, so compare line by line.
    const section =
      document.querySelector(`#${AI_SETUP_ANCHOR}`)?.textContent ?? ''
    for (const prompt of Object.values(AI_SETUP_PROMPTS.tr)) {
      for (const line of prompt.split(/\r?\n/)) {
        expect(section).toContain(line.trim())
      }
    }
    expect(section).toContain('BURAYA_ANAHTARINIZ')
  })

  it('puts the placeholder exactly once in every prompt and no real key', () => {
    for (const language of ['tr', 'en'] as const) {
      for (const prompt of Object.values(AI_SETUP_PROMPTS[language])) {
        expect(prompt.split(KEY_PLACEHOLDER)).toHaveLength(2)
        expect(prompt).not.toMatch(/sk-[A-Za-z0-9]{10,}/)
        expect(prompt).not.toContain('—')
      }
    }
  })

  it('keeps both languages of the guide complete and free of dashes', () => {
    expect(Object.keys(aiSetupTexts.en).sort()).toEqual(
      Object.keys(aiSetupTexts.tr).sort()
    )
    for (const language of ['tr', 'en'] as const) {
      const dictionary = JSON.stringify(aiSetupTexts[language]).toLowerCase()
      expect(dictionary).not.toContain('—')
      expect(dictionary).not.toMatch(/\p{Extended_Pictographic}/u)
      expect(dictionary).not.toContain('lumia')
      expect(dictionary).not.toContain('openrouter')
    }
  })

  it('scrolls to the section named in the address after loading', () => {
    const scroll = vi
      .spyOn(HTMLElement.prototype, 'scrollIntoView')
      .mockImplementation(() => {})
    window.history.replaceState(null, '', `/docs#${AI_SETUP_ANCHOR}`)

    renderDocs()

    expect(scroll).toHaveBeenCalled()
    expect(scroll.mock.contexts[0]).toBe(
      document.querySelector(`#${AI_SETUP_ANCHOR}`)
    )
  })

  it('opens the guide of the tool named in the address', () => {
    window.history.replaceState(null, '', '/docs#rehber-cline')

    renderDocs()

    expect(screen.getByRole('tab', { name: /Cline/ })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  })
})
