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
import { describe, expect, it } from 'vitest'

import { customerGroupLabel, visibleCustomerGroups } from '../customer-groups'

const turkish = ((key: string) =>
  ({ Free: 'Ücretsiz', Paid: 'Ücretli' })[key] ?? key) as unknown as TFunction

describe('customer groups', () => {
  it('hides the routing groups and keeps the groups on sale', () => {
    expect(visibleCustomerGroups(['default', 'free', 'vip', 'paid'])).toEqual([
      'free',
      'paid',
    ])
  })

  it('names the sold groups and leaves a configured group name alone', () => {
    expect(customerGroupLabel('free', turkish)).toBe('Ücretsiz')
    expect(customerGroupLabel('paid', turkish)).toBe('Ücretli')
    expect(customerGroupLabel('premium', turkish)).toBe('premium')
  })
})
