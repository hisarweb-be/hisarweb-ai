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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'

import { About } from '../index'

vi.mock('@/components/layout', () => ({
  PublicLayout: (props: { children?: React.ReactNode }) => (
    <main data-testid='public-layout'>{props.children}</main>
  ),
}))
vi.mock('../api', () => ({
  getAboutContent: vi.fn(async () => ({
    success: true,
    message: '',
    data: '',
  })),
}))

const trI18n = createInstance()
await trI18n.init({
  lng: 'tr',
  resources: { tr: { translation: {} } },
  initAsync: false,
})

describe('about page', () => {
  it('shows its own Turkish default text instead of an unset-content notice', async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <I18nextProvider i18n={trI18n}>
        <QueryClientProvider client={client}>
          <About />
        </QueryClientProvider>
      </I18nextProvider>
    )

    expect(
      await screen.findByRole('heading', { name: 'HisarWeb AI hakkında' })
    ).toBeInTheDocument()
    expect(
      screen.getByText(/Türk lirası ile yüklersiniz/)
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Durum sayfasına git' })
    ).toHaveAttribute('href', '/durum')
    expect(document.body.textContent).not.toMatch(/içeriği ayarlanmamış/i)

    const text = document.body.textContent?.toLowerCase() ?? ''
    expect(text).not.toContain('lumia')
    expect(text).not.toContain('openrouter')
  })
})
