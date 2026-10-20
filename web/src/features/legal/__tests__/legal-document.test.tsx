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

import tr from '@/i18n/locales/tr.json'

import { LegalDocument } from '../legal-document'

vi.mock('@/components/layout', () => ({
  PublicLayout: (props: { children?: React.ReactNode }) => (
    <main data-testid='public-layout'>{props.children}</main>
  ),
}))

const trI18n = createInstance()
await trI18n.init({
  lng: 'tr',
  resources: { tr },
  initAsync: false,
})

function renderDocument(
  fetchDocument: () => Promise<{ success: boolean; message: string; data: string }>
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={trI18n}>
      <QueryClientProvider client={client}>
        <LegalDocument
          title='Gizlilik Politikası'
          queryKey='privacy-policy'
          fetchDocument={fetchDocument}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

describe('legal document', () => {
  it('says in Turkish that the page will be published soon when it is empty', async () => {
    renderDocument(async () => ({ success: true, message: '', data: '' }))

    expect(
      await screen.findByText('Bu sayfa yakında yayınlanacak.')
    ).toBeInTheDocument()
  })

  it('does not repeat the administrator wording on a failed read', async () => {
    renderDocument(async () => {
      throw new Error('boom')
    })

    expect(
      await screen.findByText('Sayfa yüklenemedi. Lütfen tekrar deneyin.')
    ).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('Yönetici')
  })
})
