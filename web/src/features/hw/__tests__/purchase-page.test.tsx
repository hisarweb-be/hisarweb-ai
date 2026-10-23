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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { PurchasePage } from '../components/purchase-page'
import { hwTexts } from '../texts'
import type { HwCreditPackages } from '../types'

const { get, post, assign, getStatus } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  assign: vi.fn(),
  getStatus: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    get,
    post,
  },
  getStatus,
}))

vi.mock('@/features/hw/lib/checkout', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/features/hw/lib/checkout')>()
  return { ...actual, openCheckout: assign }
})

const tr = hwTexts.tr

const packagesAnswer: HwCreditPackages = {
  packages: [
    {
      source: 'purchase',
      granted_quota: 200_000,
      remaining_quota: 150_000,
      created_at: 1_790_000_000,
      expires_at: 1_792_592_000,
    },
  ],
  total_remaining_quota: 150_000,
  quota_per_unit: 100_000,
  try_per_usd: '42.5',
  fx_date: '2026-10-19',
  fx_source: 'Test',
}

let client: QueryClient

beforeEach(async () => {
  get.mockReset()
  post.mockReset()
  assign.mockReset()
  getStatus.mockReset()
  // The status query seeds itself from a localStorage snapshot.
  localStorage.clear()
  // A server that does not send the flag yet keeps the form open.
  getStatus.mockResolvedValue({})
  get.mockResolvedValue({
    data: { success: true, data: packagesAnswer },
  })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  await act(() => i18next.changeLanguage('tr'))
})

afterEach(() => {
  cleanup()
  client.clear()
})

// The closed-payment notice links to the start bonus, so the page needs a router.
async function renderPage(returnedFromPayment: boolean) {
  const rootRoute = createRootRoute({
    component: () => <PurchasePage returnedFromPayment={returnedFromPayment} />,
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([
      createRoute({
        getParentRoute: () => rootRoute,
        path: '/dashboard/$section',
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

/** Every text the page shows must stay in the Turkish half of the dictionary. */
function expectNoVendorName() {
  const source = document.body.outerHTML.toLowerCase()
  // Spelled in two halves so this guard itself never trips a source scan.
  expect(source).not.toContain('lu' + 'mia')
  expect(source).not.toContain('open' + 'router')
}

describe('top-up page', () => {
  it('shows the Turkish form and the packages from GET /api/user/hw/packages', async () => {
    await renderPage(false)

    expect(screen.getByText(tr.topUpPageTitle)).toBeInTheDocument()
    expect(screen.getByText(tr.topUpTitle)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: tr.topUpSubmit })).toBeEnabled()

    await screen.findByText(tr.topUpPackagesTitle)
    const line = await screen.findByText(
      new RegExp(`^${tr.packageSourcePurchase}:`)
    )
    expect(line).toHaveTextContent(/son kullanma/)
    expect(get).toHaveBeenCalledWith('/api/user/hw/packages')
    expectNoVendorName()
  })

  it('sends a whole-lira amount to POST /api/user/hw/orders and opens the checkout URL', async () => {
    const user = userEvent.setup()
    post.mockResolvedValue({
      data: {
        success: true,
        data: {
          merchant_ref: 'hw-test-1',
          checkout_url: 'https://pay.example.com/checkout/1',
        },
      },
    })
    await renderPage(false)

    const presets = screen.getByRole('group', { name: tr.topUpPresetLabel })
    await user.click(within(presets).getByRole('button', { name: /500/ }))
    await user.click(screen.getByRole('button', { name: tr.topUpSubmit }))

    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/api/user/hw/orders', {
        package_try: 500,
        language: 'tr',
      })
    )
    await waitFor(() =>
      expect(assign).toHaveBeenCalledWith('https://pay.example.com/checkout/1')
    )
  })

  it('refuses an amount outside 100-20000 TL without calling the server', async () => {
    const user = userEvent.setup()
    await renderPage(false)

    const input = screen.getByLabelText(tr.topUpCustomLabel)
    await user.clear(input)
    await user.type(input, '50')

    expect(screen.getByText(tr.topUpInvalidAmount)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: tr.topUpSubmit })).toBeDisabled()
    expect(post).not.toHaveBeenCalled()
  })

  it.each([
    [400, tr.topUpErrorRange],
    [409, tr.topUpErrorOpenOrders],
    [503, tr.topUpErrorUnavailable],
  ])('words a %i answer as %s', async (status, expected) => {
    const user = userEvent.setup()
    post.mockRejectedValue({ response: { status } })
    await renderPage(false)

    await user.click(screen.getByRole('button', { name: tr.topUpSubmit }))

    expect(await screen.findByRole('alert')).toHaveTextContent(expected)
    expect(assign).not.toHaveBeenCalled()
  })

  it('while card payment is not configured shows a calm notice and disables the order', async () => {
    getStatus.mockResolvedValue({ hw_payment_available: false })
    await renderPage(false)

    expect(await screen.findByText(tr.topUpClosedTitle)).toBeInTheDocument()
    expect(screen.getByText(tr.topUpClosedBody)).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: tr.topUpClosedBonusButton })
    ).toHaveAttribute('href', '/dashboard/overview#startbonus')
    expect(screen.getByRole('button', { name: tr.topUpSubmit })).toBeDisabled()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(post).not.toHaveBeenCalled()
  })

  it('once card payment is configured keeps the order open', async () => {
    getStatus.mockResolvedValue({ hw_payment_available: true })
    await renderPage(false)

    await waitFor(() => expect(getStatus).toHaveBeenCalled())
    expect(screen.queryByText(tr.topUpClosedTitle)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: tr.topUpSubmit })).toBeEnabled()
  })

  it('after the payment page says only that the balance follows confirmation', async () => {
    await renderPage(true)

    const alert = await screen.findByText(tr.topUpReturnPending)
    expect(alert).toBeInTheDocument()
    expect(screen.queryByText(/başarılı|onaylandı!/i)).not.toBeInTheDocument()
    expect(get).toHaveBeenCalledWith('/api/user/hw/packages')
    expectNoVendorName()
  })
})
