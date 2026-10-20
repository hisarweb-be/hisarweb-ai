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
  it('renders the Turkish guide with the base URL', () => {
    renderPage(trI18n)

    expect(screen.getByText('API belgeleri')).toBeInTheDocument()
    expect(screen.getAllByText('https://api.hisarweb.be/v1').length).toBeGreaterThan(0)
    expect(screen.getByText(/402 yetersiz bakiye/)).toBeInTheDocument()
  })

  it('renders the English guide with the base URL', () => {
    renderPage(enI18n)

    expect(screen.getByText('API documentation')).toBeInTheDocument()
    expect(screen.getAllByText('https://api.hisarweb.be/v1').length).toBeGreaterThan(0)
    expect(screen.getByText(/402 insufficient balance/)).toBeInTheDocument()
  })

  it('names no supplier anywhere on the page', () => {
    renderPage(trI18n)

    const text = document.body.textContent?.toLowerCase() ?? ''
    expect(text).not.toContain('lumia')
    expect(text).not.toContain('openrouter')
  })
})
