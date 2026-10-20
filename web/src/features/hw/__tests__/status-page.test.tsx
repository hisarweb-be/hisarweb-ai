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
import { createInstance, type i18n } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { StatusPage } from '../components/status-page'
import type { HwStatusAnswer, HwStatusState } from '../types'

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

/** Start of the sample outage: 2026-10-19 08:00 UTC. */
const incidentStart = 1792396800
const incidentEnd = incidentStart + 12 * 60

function statusAnswer(
  overall: HwStatusState,
  overrides: Partial<HwStatusAnswer> = {}
): HwStatusAnswer {
  return {
    checked_at: 1792400000,
    availability_days: 30,
    overall,
    models: [
      {
        model: 'claude-sonnet-5',
        status: 'operational',
        availability_percent: 99.95,
        message_tr: 'Tüm sistemler çalışıyor.',
        message_en: 'All systems are operational.',
      },
      {
        model: 'gpt-5.4',
        status: overall === 'operational' ? 'operational' : overall,
        availability_percent: 97.5,
        message_tr: 'Kesinti var; ekip üzerinde çalışıyor.',
        message_en: 'There is an outage; the team is working on it.',
      },
    ],
    recent: [],
    ...overrides,
  }
}

const clients: QueryClient[] = []

function renderPage(i18nInstance: i18n = trI18n) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  return render(
    <I18nextProvider i18n={i18nInstance}>
      <QueryClientProvider client={client}>
        <StatusPage />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

function mockAnswer(answer: HwStatusAnswer) {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, message: '', data: answer },
  })
}

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
})

describe('status page', () => {
  beforeEach(() => {
    mockAnswer(statusAnswer('operational'))
  })

  it('reads the public status endpoint without a login', async () => {
    renderPage()

    expect(
      await screen.findByRole('heading', { name: 'Tüm sistemler çalışıyor' })
    ).toBeInTheDocument()
    expect(api.get).toHaveBeenCalledWith('/api/hw/status')
  })

  it.each([
    ['operational', 'Tüm sistemler çalışıyor'],
    ['degraded', 'Hizmette yavaşlama var'],
    ['outage', 'Kesinti var'],
  ] as const)(
    'shows the %s state as a heading with text, not only as a colour',
    async (overall, heading) => {
      mockAnswer(statusAnswer(overall))
      renderPage()

      expect(
        await screen.findByRole('heading', { name: heading })
      ).toBeInTheDocument()
    }
  )

  it('names every model with its state and 30-day availability', async () => {
    renderPage()

    expect(await screen.findByText('claude-sonnet-5')).toBeInTheDocument()
    expect(screen.getByText('gpt-5.4')).toBeInTheDocument()
    expect(screen.getAllByText('Çalışıyor').length).toBe(2)
    expect(
      screen.getByRole('columnheader', { name: 'Son 30 gün kullanılabilirlik' })
    ).toBeInTheDocument()
    expect(screen.getByText('%99,95')).toBeInTheDocument()
    expect(screen.getByText('%97,5')).toBeInTheDocument()
  })

  it('shows no measurement instead of an invented percentage', async () => {
    mockAnswer(
      statusAnswer('operational', {
        models: [
          {
            model: 'claude-sonnet-5',
            status: 'operational',
            availability_percent: 0,
            message_tr: 'Tüm sistemler çalışıyor.',
            message_en: 'All systems are operational.',
          },
        ],
      })
    )
    renderPage()

    expect(await screen.findByText('Henüz ölçüm yok')).toBeInTheDocument()
  })

  it('shows a running and a closed outage in Turkish with time and duration', async () => {
    mockAnswer(
      statusAnswer('outage', {
        incident: {
          started_at: Math.floor(Date.now() / 1000) - 120,
          ended_at: 0,
          duration: 0,
          message_tr: 'Kesinti sürüyor; ekip üzerinde çalışıyor.',
          message_en: 'The outage is ongoing; the team is working on it.',
        },
        recent: [
          {
            started_at: incidentStart,
            ended_at: incidentEnd,
            duration: incidentEnd - incidentStart,
            message_tr: 'Kesinti sona erdi, hizmet yeniden çalışıyor.',
            message_en: 'The outage has ended and the service is back.',
          },
        ],
      })
    )
    renderPage()

    expect(await screen.findByText('Süren kesinti')).toBeInTheDocument()
    expect(
      screen.getByText('Kesinti sürüyor; ekip üzerinde çalışıyor.')
    ).toBeInTheDocument()
    expect(
      screen.getByText('Kesinti sona erdi, hizmet yeniden çalışıyor.')
    ).toBeInTheDocument()
    expect(screen.getAllByText(/Başlangıç:/).length).toBe(2)
    expect(screen.getByText(/Bitiş:/)).toBeInTheDocument()
    expect(screen.getByText(/Süre: 12 dakika/)).toBeInTheDocument()
  })

  it('shows the incidents in English after an explicit English choice', async () => {
    mockAnswer(
      statusAnswer('outage', {
        recent: [
          {
            started_at: incidentStart,
            ended_at: incidentEnd,
            duration: incidentEnd - incidentStart,
            message_tr: 'Kesinti sona erdi, hizmet yeniden çalışıyor.',
            message_en: 'The outage has ended and the service is back.',
          },
        ],
      })
    )
    renderPage(enI18n)

    expect(await screen.findByText('Recent outages')).toBeInTheDocument()
    expect(
      screen.getByText('The outage has ended and the service is back.')
    ).toBeInTheDocument()
    expect(screen.getByText(/Duration: 12 min/)).toBeInTheDocument()
    expect(screen.queryByText(/dakika/)).not.toBeInTheDocument()
  })

  it('says so when no outage was recorded', async () => {
    renderPage()

    expect(
      await screen.findByText('Bu dönemde kayıtlı kesinti yok.')
    ).toBeInTheDocument()
  })

  it('explains that a long outage extends paid packages', async () => {
    renderPage()

    expect(
      await screen.findByText('Kesinti uzarsa bakiyeniz de uzar')
    ).toBeInTheDocument()
    expect(screen.getByText(/15 dakikadan uzun süren bir kesintide/)).toBeInTheDocument()
  })

  it('shows an empty state when the answer carries no models', async () => {
    mockAnswer(statusAnswer('operational', { models: [] }))
    renderPage()

    expect(await screen.findByText('Henüz durum verisi yok')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('reports a failed read in Turkish without a table', async () => {
    vi.spyOn(api, 'get').mockRejectedValue(
      Object.assign(new Error('Server error'), { response: { status: 500 } })
    )
    renderPage()

    expect(
      await screen.findByText('Durum bilgisi okunamadı. Lütfen tekrar deneyin.')
    ).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('names no upstream supplier', async () => {
    mockAnswer(
      statusAnswer('outage', {
        recent: [
          {
            started_at: incidentStart,
            ended_at: incidentEnd,
            duration: incidentEnd - incidentStart,
            message_tr: 'Kesinti sona erdi, hizmet yeniden çalışıyor.',
            message_en: 'The outage has ended and the service is back.',
          },
        ],
      })
    )
    renderPage()
    await screen.findByText('claude-sonnet-5')

    const text = document.body.textContent?.toLowerCase() ?? ''
    expect(text).not.toContain('lumia')
    expect(text).not.toContain('openrouter')
  })
})
