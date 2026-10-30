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
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

const post = vi.hoisted(() => vi.fn())

vi.mock('axios', () => ({
  default: {
    create: () => ({ post }),
    isAxiosError: (error: unknown) =>
      Boolean((error as { isAxiosError?: boolean })?.isAxiosError),
  },
}))

// The page itself is irrelevant here; only the route guard is under test.
vi.mock('@/features/auth/sign-in', () => ({ SignIn: () => null }))

const { Route } = await import('@/routes/(auth)/sign-in')

const HINT_COOKIE = 'new_api_has_session'

const bundle: AuthBundle = {
  access_token: 'access-token',
  token_type: 'Bearer',
  access_expires_at: Math.floor(Date.now() / 1000) + 600,
  user: { id: 42, username: 'test-user', role: 1 },
  session: {
    sid: 'session-a',
    current: true,
    login_method: 'password',
    ip: '127.0.0.1',
    user_agent: 'test',
    created_at: 100,
    last_active_at: 100,
    expires_at: 1000,
  },
}

function runBeforeLoad() {
  const beforeLoad = Route.options.beforeLoad as (ctx: {
    search: { redirect?: string }
  }) => Promise<void>
  return beforeLoad({ search: {} })
}

beforeEach(() => {
  post.mockReset()
  useAuthStore.getState().auth.reset('idle')
  document.cookie = `${HINT_COOKIE}=; Max-Age=0; path=/`
})

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
  document.cookie = `${HINT_COOKIE}=; Max-Age=0; path=/`
})

describe('sign-in route session check', () => {
  test('an anonymous visitor without a session hint triggers no refresh request', async () => {
    await runBeforeLoad()

    expect(post).not.toHaveBeenCalled()
    expect(useAuthStore.getState().auth.user).toBeNull()
  })

  test('a visitor with a session hint still gets the session restored', async () => {
    document.cookie = `${HINT_COOKIE}=1; path=/`
    post.mockResolvedValue({
      status: 200,
      data: { success: true, data: bundle },
    })

    await expect(runBeforeLoad()).rejects.toMatchObject({
      options: { href: '/dashboard' },
    })

    expect(post).toHaveBeenCalledTimes(1)
    expect(post.mock.calls[0][0]).toBe('/api/user/auth/refresh')
    expect(useAuthStore.getState().auth.user?.username).toBe('test-user')
  })

  test('a stale hint is answered by the server and ends as anonymous', async () => {
    document.cookie = `${HINT_COOKIE}=1; path=/`
    post.mockRejectedValue({
      isAxiosError: true,
      response: { status: 401, data: { success: false } },
    })

    await runBeforeLoad()

    expect(post).toHaveBeenCalledTimes(1)
    expect(useAuthStore.getState().auth.user).toBeNull()
  })
})
