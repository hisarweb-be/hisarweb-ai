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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { PartnerChannels } from '..'

const channel = {
  user_id: 7,
  username: 'partner7',
  display_name: 'Partner Seven',
  granted_at: 1_700_000_000,
  expires_at: 0,
  signups: 3,
  paying_customers: 1,
  earned_quota: 0,
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <PartnerChannels />
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('lists the partner channels', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [channel] },
  })
  renderPage()
  expect(await screen.findByText('partner7')).toBeInTheDocument()
  expect(screen.getByText('Partner Seven')).toBeInTheDocument()
})

it('grants a channel to the typed user id and reloads the list', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [] },
  })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: { credit_granted: true, bonus_granted: true, bonus_pending: false },
    },
  })
  renderPage()
  await screen.findByText('No channels granted yet.')
  const button = screen.getByRole('button', { name: 'Grant channel' })
  expect(button).toBeDisabled()
  await userEvent.type(screen.getByLabelText('User ID'), '7')
  await userEvent.click(button)
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith('/api/hw/admin/channel-grant', {
      user_id: 7,
    })
  )
  await waitFor(() => expect(get).toHaveBeenCalledTimes(2))
})

it('shows an error state when the list fails', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: false, message: 'nope' },
  })
  renderPage()
  expect(
    await screen.findByText('We could not load the channels.')
  ).toBeInTheDocument()
})
