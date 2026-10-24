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
import { ROLE } from '@/lib/roles'

import type { NavGroup, TopNavLink } from '../types'

/** Header links that only administrators and developers need. */
const ADMIN_ONLY_TOP_NAV_HREFS: ReadonlySet<string> = new Set([
  '/dashboard',
  '/pricing',
  '/rankings',
  '/about',
])

/**
 * Narrow the root sidebar by role: the admin group is for administrators
 * only, and an item with `requiredRole` is hidden below that role.
 */
export function filterNavGroupsByRole(
  groups: NavGroup[],
  role: number
): NavGroup[] {
  const isAdmin = role >= ROLE.ADMIN
  return groups
    .filter((group) => (group.id === 'admin' ? isAdmin : true))
    .map((group) => {
      const items = group.items.filter(
        (item) => item.requiredRole === undefined || role >= item.requiredRole
      )
      return items.length === group.items.length ? group : { ...group, items }
    })
    .filter((group) => group.items.length > 0)
}

/**
 * Beginner mode for the signed-in header: ordinary customers (role below
 * ADMIN) do not get the console, model catalogue, rankings or about links.
 */
export function filterTopNavLinksByRole<T extends Pick<TopNavLink, 'href'>>(
  links: T[],
  role: number
): T[] {
  if (role >= ROLE.ADMIN) return links
  return links.filter((link) => !ADMIN_ONLY_TOP_NAV_HREFS.has(link.href))
}
