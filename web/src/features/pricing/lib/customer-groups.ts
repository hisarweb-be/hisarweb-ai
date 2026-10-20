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
import type { TFunction } from 'i18next'

/**
 * Groups that exist for routing only and are not sold on the public price
 * list: `default` is the fallback for a request without a group and `vip` is
 * an internal arrangement. A customer can neither pick nor buy them, so the
 * public price list does not name them.
 */
export const HIDDEN_CUSTOMER_GROUPS = ['default', 'vip']

/** The groups of a model or of the catalog that a customer may see. */
export function visibleCustomerGroups(groups: string[]): string[] {
  return groups.filter((group) => !HIDDEN_CUSTOMER_GROUPS.includes(group))
}

/**
 * The label of a sold group. `free` and `paid` are the two groups on sale, so
 * they read as words; any other group keeps its configured name.
 */
export function customerGroupLabel(group: string, t: TFunction): string {
  if (group === 'free') {
    return t('Free')
  }
  if (group === 'paid') {
    return t('Paid')
  }
  return group
}
