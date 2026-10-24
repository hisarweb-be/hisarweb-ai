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
import { describe, expect, it } from 'vitest'

import { ROLE, isOrdinaryCustomer } from '@/lib/roles'

import type { NavGroup } from '../../types'
import {
  filterNavGroupsByRole,
  filterTopNavLinksByRole,
} from '../role-visibility'

const GROUPS: NavGroup[] = [
  {
    id: 'general',
    title: 'General',
    items: [
      { title: 'Keys', url: '/keys' },
      { title: 'Dashboard', url: '/dashboard', requiredRole: ROLE.ADMIN },
    ],
  },
  {
    id: 'admin',
    title: 'Admin',
    items: [{ title: 'Channels', url: '/channels' }],
  },
  {
    id: 'only-admin-items',
    title: 'Logs',
    items: [{ title: 'Audit', url: '/audit', requiredRole: ROLE.ADMIN }],
  },
]

const LINKS = [
  { title: 'Home', href: '/' },
  { title: 'Console', href: '/dashboard' },
  { title: 'Models', href: '/pricing' },
  { title: 'Rankings', href: '/rankings' },
  { title: 'About', href: '/about' },
  { title: 'Docs', href: '/docs' },
]

describe('filterNavGroupsByRole', () => {
  it('hides the admin group and admin items for an ordinary customer', () => {
    const result = filterNavGroupsByRole(GROUPS, ROLE.USER)

    expect(result.map((group) => group.id)).toEqual(['general'])
    expect(result[0].items.map((item) => item.title)).toEqual(['Keys'])
  })

  it('keeps everything for an administrator', () => {
    const result = filterNavGroupsByRole(GROUPS, ROLE.ADMIN)

    expect(result).toEqual(GROUPS)
  })

  it('removes groups that end up empty', () => {
    const result = filterNavGroupsByRole(GROUPS, ROLE.USER)

    expect(result.some((group) => group.id === 'only-admin-items')).toBe(false)
  })
})

describe('filterTopNavLinksByRole', () => {
  it('drops the console, catalogue, rankings and about links for customers', () => {
    const hrefs = filterTopNavLinksByRole(LINKS, ROLE.USER).map(
      (link) => link.href
    )

    expect(hrefs).toEqual(['/', '/docs'])
  })

  it('keeps all links for an administrator', () => {
    expect(filterTopNavLinksByRole(LINKS, ROLE.ADMIN)).toEqual(LINKS)
  })
})

describe('isOrdinaryCustomer', () => {
  it('is true only for roles between user and admin', () => {
    expect(isOrdinaryCustomer(undefined)).toBe(false)
    expect(isOrdinaryCustomer(ROLE.GUEST)).toBe(false)
    expect(isOrdinaryCustomer(ROLE.USER)).toBe(true)
    expect(isOrdinaryCustomer(ROLE.ADMIN)).toBe(false)
    expect(isOrdinaryCustomer(ROLE.SUPER_ADMIN)).toBe(false)
  })
})
