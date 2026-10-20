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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { QualityMeter } from '../components/quality-meter'
import type { HwQualityLatest } from '../types'

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

const clients: QueryClient[] = []

const sampleNote =
  'Bu ölçümler sabit ve sınırlı bir görev setiyle yapılan bir örneklemdir, garanti değildir.'

const measured: HwQualityLatest = {
  measured_at: 1792400000,
  file: 'run-1.json',
  taskset_version: 'v1',
  task_count: 12,
  judge_model: 'judge',
  models: [
    {
      model: 'claude-sonnet-5',
      reference: 'ref',
      avg_score: 7.25,
      avg_latency_ms: 1850,
      tasks: [],
    },
    {
      model: 'gpt-5.4-mini',
      reference: 'ref',
      avg_score: 6,
      avg_latency_ms: 900,
      tasks: [],
    },
  ],
  note: sampleNote,
  note_en:
    'These measurements are a sample over a small fixed task set, not a guarantee.',
}

const empty: HwQualityLatest = {
  measured_at: 0,
  file: '',
  taskset_version: '',
  task_count: 0,
  judge_model: '',
  models: [],
  note: sampleNote,
  note_en:
    'These measurements are a sample over a small fixed task set, not a guarantee.',
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  return render(
    <I18nextProvider i18n={trI18n}>
      <QueryClientProvider client={client}>
        <QualityMeter />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, message: '', data: measured },
  })
})

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
})

describe('quality meter page', () => {
  it('reads the latest measurement from the public endpoint', async () => {
    renderPage()

    expect(await screen.findByText('claude-sonnet-5')).toBeInTheDocument()
    expect(api.get).toHaveBeenCalledWith('/api/hw/quality/latest')
  })

  it('shows score, latency and measurement date per model', async () => {
    renderPage()

    expect(await screen.findByText('claude-sonnet-5')).toBeInTheDocument()
    expect(screen.getByText('7,25')).toBeInTheDocument()
    expect(screen.getByText('1.850 ms')).toBeInTheDocument()
    expect(screen.getByText('gpt-5.4-mini')).toBeInTheDocument()
    expect(screen.getAllByText('Ölçüm tarihi').length).toBeGreaterThan(0)
  })

  it('shows the API sample note that the results are a sample', async () => {
    renderPage()

    expect(await screen.findByText(sampleNote)).toBeInTheDocument()
  })

  it('shows an empty state without a measurement and invents no values', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, message: '', data: empty },
    })
    renderPage()

    expect(await screen.findByText('İlk ölçüm yakında')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Henüz yayınlanmış bir ölçüm yok. İlk ölçüm tamamlandığında sonuçlar burada görünecek.'
      )
    ).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('reports a failed read in Turkish without a table', async () => {
    vi.spyOn(api, 'get').mockRejectedValue(
      Object.assign(new Error('Server error'), { response: { status: 500 } })
    )
    renderPage()

    expect(
      await screen.findByText('Ölçüm verisi okunamadı. Lütfen tekrar deneyin.')
    ).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('never claims the models equal the original and names no supplier', async () => {
    renderPage()
    await screen.findByText('claude-sonnet-5')

    const text = document.body.textContent?.toLowerCase() ?? ''
    expect(text).not.toContain('lumia')
    expect(text).not.toContain('openrouter')
    expect(text).not.toContain('orijinal')
    expect(text).not.toContain('gelijk aan')
  })
})
