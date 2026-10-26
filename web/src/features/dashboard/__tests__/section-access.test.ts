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
import { isRedirect } from '@tanstack/react-router'
import { afterEach, describe, expect, it } from 'vitest'

import { ROLE } from '@/lib/roles'
import { Route } from '@/routes/_authenticated/dashboard/$section'
import { useAuthStore } from '@/stores/auth-store'

type SectionBeforeLoad = (ctx: { params: { section: string } }) => void

function openSection(section: string, role: number): unknown {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'alice', role })
  try {
    ;(Route.options.beforeLoad as unknown as SectionBeforeLoad)({
      params: { section },
    })
    return null
  } catch (error) {
    return error
  }
}

afterEach(() => {
  useAuthStore.setState(useAuthStore.getInitialState(), true)
})

describe('dashboard section access', () => {
  it('redirects a customer who opens the admin-only users section by URL', () => {
    const result = openSection('users', ROLE.USER)
    expect(isRedirect(result)).toBe(true)
  })

  it('lets an administrator open the users section', () => {
    expect(openSection('users', ROLE.ADMIN)).toBeNull()
  })

  it('lets a customer open the sections backed by self data', () => {
    expect(openSection('overview', ROLE.USER)).toBeNull()
    expect(openSection('models', ROLE.USER)).toBeNull()
  })
})
