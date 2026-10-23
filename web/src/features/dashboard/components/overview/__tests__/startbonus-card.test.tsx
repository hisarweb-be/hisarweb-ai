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
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { Toaster, toast } from 'sonner'
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
/** Answer of `POST /api/hw/telegram/link`; each test overrides what it needs. */
let linkAnswer: Record<string, unknown>
/** Answer of `GET /api/hw/telegram/link`, how far the bot conversation is. */
let linkStatus: {
  linked: boolean
  phone_shared: boolean
  bonus_granted: boolean
}
/** Every GET the card issued, so a refetch can be told apart from a first load. */
const getCalls: string[] = []

const ONE_TIME_LINK = 'https://t.me/HisarWebAI_bot?start=one-time-token'
const TEXT_LINKED =
  'Telegram bağlandı. Şimdi bottaki Numaramı paylaş düğmesine dokunun.'

beforeEach(async () => {
  await useLanguage('tr')
  getCalls.length = 0
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  useAuthStore.getState().auth.setUser({ id: 1, username: 'new-user', role: 1 })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  status = { telegram_oauth_configured: false }
  packages = { packages: [], phone_shared: false }
  linkAnswer = {
    success: true,
    data: { linked: false, url: ONE_TIME_LINK, expires_at: 1_770_000_000 },
  }
  linkStatus = { linked: false, phone_shared: false, bonus_granted: false }
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    getCalls.push(url)
    switch (url) {
      case '/api/status':
        return { data: { success: true, data: status } }
      case '/api/user/self':
        return {
          data: {
            success: true,
            data: { id: 1, username: 'new-user', role: 1 },
          },
        }
      case '/api/hw/telegram/link':
        return { data: { success: true, data: linkStatus } }
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
  vi.spyOn(api, 'post').mockImplementation(async () => ({ data: linkAnswer }))
})

afterEach(async () => {
  cleanup()
  client.clear()
  toast.dismiss()
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
      <Toaster />
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

    const telegramStep = await stepItem('Telegram hesabınızı bağlayın')
    expect(telegramStep.getByText('Henüz açık değil')).toBeVisible()
    expect(
      telegramStep.queryByRole('button', { name: 'Link Telegram' })
    ).not.toBeInTheDocument()

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

  it('opens the bot dialog from the phone step once telegram verification is open', async () => {
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
    linkStatus = { linked: true, phone_shared: false, bonus_granted: false }
    await renderCard()

    const phoneStep = await stepItem(
      'Telefon numaranızı Telegram botuyla paylaşın'
    )
    expect(phoneStep.getByText('Henüz yapılmadı')).toBeVisible()
    const user = userEvent.setup()
    await user.click(
      phoneStep.getByRole('button', { name: 'Telegram botunu aç' })
    )

    const dialog = within(await screen.findByRole('dialog'))
    expect(api.post).toHaveBeenCalledWith('/api/hw/telegram/link')
    expect(await dialog.findByText(TEXT_LINKED)).toBeVisible()
  })

  it('opens both telegram steps when only the bot name is configured', async () => {
    status = {
      telegram_oauth_configured: false,
      telegram_bot_name: '@HisarWebAI_bot',
    }
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'new-user',
      role: 1,
      email: 'new-user@example.com',
    })
    await renderCard()

    const telegramStep = await stepItem('Telegram hesabınızı bağlayın')
    expect(telegramStep.getByText('Henüz yapılmadı')).toBeVisible()
    expect(
      telegramStep.getByRole('button', { name: 'Link Telegram' })
    ).toBeVisible()

    const phoneStep = await stepItem(
      'Telefon numaranızı Telegram botuyla paylaşın'
    )
    expect(phoneStep.getByText('Henüz yapılmadı')).toBeVisible()
    expect(
      phoneStep.getByRole('button', { name: 'Telegram botunu aç' })
    ).toBeVisible()
  })

  it('offers telegram web, the app and the t.me link for the one-time link', async () => {
    status = { telegram_bot_name: '@HisarWebAI_bot' }
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'new-user',
      role: 1,
      email: 'new-user@example.com',
    })
    await renderCard()

    const telegramStep = await stepItem('Telegram hesabınızı bağlayın')
    const user = userEvent.setup()
    await user.click(
      telegramStep.getByRole('button', { name: 'Link Telegram' })
    )

    const dialog = within(await screen.findByRole('dialog'))
    expect(
      await dialog.findByRole('button', { name: 'Telegram Web’de aç' })
    ).toHaveAttribute(
      'href',
      'https://web.telegram.org/k/#?tgaddr=tg%3A%2F%2Fresolve%3Fdomain%3DHisarWebAI_bot%26start%3Done-time-token'
    )
    expect(
      dialog.getByRole('button', { name: 'Telegram uygulamasında aç' })
    ).toHaveAttribute(
      'href',
      'tg://resolve?domain=HisarWebAI_bot&start=one-time-token'
    )
    expect(dialog.getByRole('link', { name: ONE_TIME_LINK })).toHaveAttribute(
      'href',
      ONE_TIME_LINK
    )
    expect(dialog.getByText('Telegram’dan onay bekleniyor…')).toBeVisible()
  })

  it('says the bonus arrived and keeps the dialog open once the bot granted it', async () => {
    status = { telegram_bot_name: '@HisarWebAI_bot' }
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'new-user',
      role: 1,
      email: 'new-user@example.com',
    })
    await renderCard()

    const telegramStep = await stepItem('Telegram hesabınızı bağlayın')
    linkStatus = { linked: true, phone_shared: true, bonus_granted: true }
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
    const user = userEvent.setup()
    await user.click(
      telegramStep.getByRole('button', { name: 'Link Telegram' })
    )

    const dialog = within(await screen.findByRole('dialog'))
    expect(
      await dialog.findByText('Bağlandı, 250 TL bonus hesabınızda')
    ).toBeVisible()
    await waitFor(() =>
      expect(
        screen.queryByRole('heading', {
          name: '250 TL başlangıç bonusunuzu alın',
        })
      ).not.toBeInTheDocument()
    )
    expect(screen.getByRole('dialog')).toBeVisible()
  })

  it('shows the server message when the link cannot be created', async () => {
    status = { telegram_bot_name: '@HisarWebAI_bot' }
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'new-user',
      role: 1,
      email: 'new-user@example.com',
    })
    linkAnswer = {
      success: false,
      message: 'Failed to create the Telegram link',
    }
    await renderCard()

    const telegramStep = await stepItem('Telegram hesabınızı bağlayın')
    const user = userEvent.setup()
    await user.click(
      telegramStep.getByRole('button', { name: 'Link Telegram' })
    )

    expect(
      await screen.findByText('Failed to create the Telegram link')
    ).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Telegram Web’de aç' })
    ).not.toBeInTheDocument()
  })

  it('reads the account and the packages again when the window regains focus', async () => {
    status = { telegram_bot_name: '@HisarWebAI_bot' }
    await renderCard()
    await stepItem('Telegram hesabınızı bağlayın')

    getCalls.length = 0
    await act(async () => {
      window.dispatchEvent(new Event('focus'))
    })

    await waitFor(() => expect(getCalls).toContain('/api/user/self'))
    expect(getCalls).toContain('/api/user/hw/packages')
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
