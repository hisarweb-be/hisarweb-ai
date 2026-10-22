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
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'

import { DocsPage } from '../components/docs-page'

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

const enI18n = createInstance()
await enI18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

function renderPage(i18n: typeof trI18n) {
  return render(
    <I18nextProvider i18n={i18n}>
      <DocsPage />
    </I18nextProvider>
  )
}

describe('documentation page', () => {
  it('opens the Turkish developer reference with the base URL', async () => {
    renderPage(trI18n)

    await userEvent.click(
      screen.getByRole('button', { name: 'Geliştiriciler için' })
    )

    expect(screen.getByText('API belgeleri')).toBeInTheDocument()
    expect(
      screen.getAllByText('https://api.hisarweb.be/v1').length
    ).toBeGreaterThan(0)
    expect(screen.getByText(/402 yetersiz bakiye/)).toBeInTheDocument()
  })

  it('opens the English developer reference with the base URL', async () => {
    renderPage(enI18n)

    await userEvent.click(
      screen.getByRole('button', { name: 'For developers' })
    )

    expect(screen.getByText('API documentation')).toBeInTheDocument()
    expect(
      screen.getAllByText('https://api.hisarweb.be/v1').length
    ).toBeGreaterThan(0)
    expect(screen.getByText(/402 insufficient balance/)).toBeInTheDocument()
  })

  it('shows the six beginner steps with a screenshot for each visual step', () => {
    renderPage(trI18n)

    expect(screen.getByText('HisarWeb AI nedir?')).toBeInTheDocument()
    for (const title of [
      'Hesap açın',
      '250 TL başlangıç bonusunu alın',
      'API anahtarı oluşturun',
      'Bakiye yükleyin (isteğe bağlı)',
      'Playground’da deneyin',
      'Claude Code’u HisarWeb AI’ye bağlayın',
    ]) {
      expect(screen.getByRole('heading', { name: title })).toBeInTheDocument()
    }
    expect(screen.getAllByRole('img')).toHaveLength(5)
    expect(
      screen.getByRole('link', { name: 'Satın al sayfasına git' })
    ).toHaveAttribute('href', '/satin-al')
  })

  it('gives Claude Code the gateway address without a trailing /v1', () => {
    renderPage(trI18n)

    const page = document.body.textContent ?? ''
    expect(page).toContain('"ANTHROPIC_BASE_URL": "https://api.hisarweb.be"')
    expect(page).not.toContain(
      '"ANTHROPIC_BASE_URL": "https://api.hisarweb.be/v1"'
    )
  })

  it('lists the Claude Desktop steps after switching tabs', async () => {
    renderPage(enI18n)

    await userEvent.click(screen.getByRole('tab', { name: 'Desktop app' }))

    expect(
      screen.getByText(/Configure Third-Party Inference/)
    ).toBeInTheDocument()
    expect(screen.getByText(/Save & Restart/)).toBeInTheDocument()
  })

  it('names no supplier anywhere on the page', async () => {
    renderPage(trI18n)
    await userEvent.click(
      screen.getByRole('button', { name: 'Geliştiriciler için' })
    )

    const text = document.body.textContent?.toLowerCase() ?? ''
    expect(text).not.toContain('lumia')
    expect(text).not.toContain('openrouter')
  })
})
