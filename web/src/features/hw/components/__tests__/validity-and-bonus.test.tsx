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
import { cleanup, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, describe, expect, it } from 'vitest'

import type { HwBonusRule, HwCreditValidity } from '../../types'
import { hwBonusFor } from '../../lib/bonus'
import { HwBonusBadge, HwOrderBonusBadge } from '../bonus-badge'
import { CreditValidityCard } from '../credit-validity-card'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

afterEach(cleanup)

function withI18n(node: React.ReactNode) {
  return render(<I18nextProvider i18n={i18n}>{node}</I18nextProvider>)
}

const active = (days: number): HwCreditValidity => ({
  status: 'active',
  valid_until: 1_800_000_000,
  days_remaining: days,
  expired_at: 0,
  expired_quota: 0,
})

describe('CreditValidityCard', () => {
  it('counts down the days and stays calm above three days', () => {
    withI18n(<CreditValidityCard validity={active(12)} locale='en' />)
    expect(screen.getByText('Credit validity: 12 days left')).toBeTruthy()
    expect(screen.getByLabelText('Credit validity').dataset.state).toBe('ok')
  })

  it.each([3, 1])('turns red at %i days', (days) => {
    withI18n(<CreditValidityCard validity={active(days)} locale='en' />)
    expect(screen.getByLabelText('Credit validity').dataset.state).toBe(
      'urgent'
    )
  })

  it('turns red and names the lost balance once expired', () => {
    withI18n(
      <CreditValidityCard
        locale='en'
        validity={{
          status: 'expired',
          valid_until: 0,
          days_remaining: 0,
          expired_at: 1_790_000_000,
          expired_quota: 500,
        }}
      />
    )
    const card = screen.getByLabelText('Credit validity')
    expect(card.dataset.state).toBe('expired')
    expect(screen.getByText('Credit validity: expired')).toBeTruthy()
    expect(card.textContent).toContain('was removed from the balance')
  })

  it('draws nothing without credit or on an older server', () => {
    const { container, rerender } = withI18n(
      <CreditValidityCard
        validity={{ ...active(0), status: 'none', valid_until: 0 }}
      />
    )
    expect(container.innerHTML).toBe('')
    rerender(
      <I18nextProvider i18n={i18n}>
        <CreditValidityCard />
      </I18nextProvider>
    )
    expect(container.innerHTML).toBe('')
  })
})

describe('bonus badge', () => {
  const rules: HwBonusRule[] = [
    { currency: 'TRY', min: 500, amount: 50 },
    { currency: 'USD', min: 10, amount: 1 },
    { currency: 'EUR', min: 0, amount: 0 },
  ]

  it('applies the rule from the settings at the boundary', () => {
    expect(hwBonusFor(rules, 'TRY', 500)).toBe(50)
    expect(hwBonusFor(rules, 'TRY', 499)).toBe(0)
    expect(hwBonusFor(rules, 'USD', 10)).toBe(1)
    expect(hwBonusFor(rules, 'EUR', 100)).toBe(0)
    expect(hwBonusFor(undefined, 'TRY', 1000)).toBe(0)
  })

  it('shows +₺50 bonus and nothing for zero', () => {
    const { container } = withI18n(
      <HwBonusBadge currency='TRY' amount={50} locale='tr-TR' />
    )
    expect(container.textContent).toContain('+₺50 bonus')
    cleanup()
    const empty = withI18n(<HwBonusBadge currency='TRY' amount={0} />)
    expect(empty.container.innerHTML).toBe('')
  })

  it('reads the stored minor units of an order', () => {
    const { container } = withI18n(
      <HwOrderBonusBadge currency='USD' bonusMinor={100} locale='en-US' />
    )
    expect(container.textContent).toContain('+$1 bonus')
  })
})
