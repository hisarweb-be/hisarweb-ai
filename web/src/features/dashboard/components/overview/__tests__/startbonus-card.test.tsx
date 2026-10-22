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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import i18next from 'i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { hwPackagesQueryKey } from '@/features/hw/hooks/use-hw-packages'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { StartbonusCard } from '../startbonus-card'

// The card reads its dictionary through the resolved i18next language, so the
// tests switch that language the same way the header's language switcher does.
async function useLanguage(language: string) {
  i18next.addResourceBundle(language, 'translation', {})
  await i18next.changeLanguage(language)
}

type StatusFixture = {
  telegram_oauth_configured?: boolean
  telegram_bot_name?: string
}

type PackagesFixture = {
  packages: {
    source: string
    granted_quota: number
    remaining_quota: number
    created_at: number
    expires_at: number
  }[]
  phone_shared: boolean
}

let client: QueryClient
let status: StatusFixture
let packages: PackagesFixture

beforeEach(async () => {
  await useLanguage('tr')
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  useAuthStore.getState().auth.setUser({ id: 1, username: 'new-user', role: 1 })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  status = { telegram_oauth_configured: false }
  packages = { packages: [], phone_shared: false }
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    switch (url) {
      case '/api/status':
        return { data: { success: true, data: status } }
      case '/api/user/hw/packages':
        return {
          data: {
            success: true,
            data: {
              ...packages,
              total_remaining_quota: 0,
              quota_per_unit: 500000,
              try_per_usd: '',
              fx_date: '',
              fx_source: '',
            },
          },
        }
      default:
        throw new Error(`Unexpected dashboard request: ${url}`)
    }
  })
})

afterEach(async () => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  await useLanguage('en')
})

async function renderCard() {
  const rootRoute = createRootRoute({ component: StartbonusCard })
  const router = createRouter({
    routeTree: rootRoute.addChildren([
      createRoute({
        getParentRoute: () => rootRoute,
        path: '/security',
        component: () => null,
      }),
      createRoute({
        getParentRoute: () => rootRoute,
        path: '/satin-al',
        component: () => null,
      }),
    ]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

function stepItem(title: string) {
  return screen.findByText(title).then((node) => {
    const item = node.closest('li')
    if (!item) throw new Error(`No list item around ${title}`)
    return within(item)
  })
}

// The shared Button renders as an anchor with `role="button"`, so every call to
// action is found by that role and its destination is read from `href`.
describe('start bonus card', () => {
  it('offers the e-mail step while telegram verification is closed', async () => {
    await renderCard()

    const emailStep = await stepItem('E-posta adresinizi ekleyin ve onaylayın')
    expect(emailStep.getByText('Henüz yapılmadı')).toBeVisible()
    expect(
      emailStep.getByRole('button', { name: 'E-postayı ekle' })
    ).toHaveAttribute('href', '/security')

    const phoneStep = await stepItem(
      'Telefon numaranızı Telegram botuyla paylaşın'
    )
    expect(phoneStep.getByText('Henüz açık değil')).toBeVisible()
    expect(
      phoneStep.queryByRole('button', { name: 'Telegram botunu aç' })
    ).not.toBeInTheDocument()

    // The headline action keeps pointing at the e-mail step, even though the
    // Telegram part of the bonus cannot be done yet.
    const actions = screen.getAllByRole('button', { name: 'E-postayı ekle' })
    expect(actions).toHaveLength(2)
    for (const action of actions) {
      expect(action).toHaveAttribute('href', '/security')
    }
  })

  it('links the phone step to the bot once telegram verification is open', async () => {
    status = {
      telegram_oauth_configured: true,
      telegram_bot_name: '@HisarWebAI_bot',
    }
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'new-user',
      role: 1,
      email: 'new-user@example.com',
      telegram_id: 'telegram-user',
    })
    await renderCard()

    const telegramStep = await stepItem(
      'Telefon numaranızı Telegram botuyla paylaşın'
    )
    expect(telegramStep.getByText('Henüz yapılmadı')).toBeVisible()
    expect(
      telegramStep.getByRole('button', { name: 'Telegram botunu aç' })
    ).toHaveAttribute('href', 'https://t.me/HisarWebAI_bot')
  })

  it('hides the card once the free package exists', async () => {
    packages = {
      packages: [
        {
          source: 'free',
          granted_quota: 250000,
          remaining_quota: 250000,
          created_at: 1_770_000_000,
          expires_at: 1_771_209_600,
        },
      ],
      phone_shared: true,
    }
    await renderCard()

    await waitFor(() =>
      expect(client.getQueryData(hwPackagesQueryKey)).toBeDefined()
    )
    await act(async () => {})
    expect(
      screen.queryByRole('heading', {
        name: '250 TL başlangıç bonusunuzu alın',
      })
    ).not.toBeInTheDocument()
    expect(document.querySelector('#startbonus')).toBeNull()
  })
})
