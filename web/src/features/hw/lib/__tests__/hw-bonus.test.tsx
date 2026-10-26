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
import {
  act,
  cleanup,
  render,
  renderHook,
  screen,
} from '@testing-library/react'
import i18next from 'i18next'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { useHwBonusStore } from '@/stores/hw-bonus-store'
import { useHwCurrencyStore } from '@/stores/hw-currency-store'

import { BonusExplainer } from '../../components/bonus-explainer'
import { hwTexts } from '../../texts'
import { HW_LEGACY_CURRENCY_CONFIG } from '../currency'
import {
  FALLBACK_BONUS_RULES,
  FALLBACK_VALIDITY_DAYS,
  parseHwBonusConfig,
  parseHwBonusRules,
  resolveHwBonus,
  useHwBonusRules,
} from '../hw-bonus'

beforeEach(async () => {
  useHwCurrencyStore.setState({ config: HW_LEGACY_CURRENCY_CONFIG })
  useHwBonusStore.setState({
    status: { rules: null, validityDays: null },
    packageRules: null,
  })
  i18next.addResourceBundle('tr', 'translation', {})
  await act(() => i18next.changeLanguage('tr'))
})

afterEach(cleanup)

describe('parseHwBonusConfig', () => {
  it('reads the public status fields', () => {
    expect(
      parseHwBonusConfig({
        hw_bonus_rules: [{ currency: 'EUR', min: 20, amount: 3 }],
        hw_credit_validity_days: 45,
      })
    ).toEqual({
      rules: [{ currency: 'EUR', min: 20, amount: 3 }],
      validityDays: 45,
    })
  })

  it('leaves absent or malformed fields empty so the fallback applies', () => {
    expect(parseHwBonusConfig({})).toEqual({ rules: null, validityDays: null })
    expect(parseHwBonusConfig(null)).toEqual({
      rules: null,
      validityDays: null,
    })
    expect(
      parseHwBonusConfig({ hw_bonus_rules: 'x', hw_credit_validity_days: -1 })
    ).toEqual({ rules: null, validityDays: null })
  })

  it('drops rules that are off (amount 0) or invalid', () => {
    expect(
      parseHwBonusRules([
        { currency: 'USD', min: 10, amount: 0 },
        { currency: 'GBP', min: 10, amount: 1 },
        { currency: 'TRY', min: 500, amount: 50 },
      ])
    ).toEqual([{ currency: 'TRY', min: 500, amount: 50 }])
  })
})

describe('useHwBonusRules', () => {
  it('falls back to the named constants until the API delivers the fields', () => {
    const { result } = renderHook(() => useHwBonusRules())

    expect(result.current.rules).toBe(FALLBACK_BONUS_RULES)
    expect(result.current.validityDays).toBe(FALLBACK_VALIDITY_DAYS)
  })

  it('prefers the API values over the fallback', () => {
    useHwBonusStore.setState({
      status: {
        rules: [{ currency: 'TRY', min: 1000, amount: 120 }],
        validityDays: 90,
      },
    })
    const { result } = renderHook(() => useHwBonusRules())

    expect(result.current.rules).toEqual([
      { currency: 'TRY', min: 1000, amount: 120 },
    ])
    expect(result.current.validityDays).toBe(90)
  })

  it('prefers the signed-in rules of the packages answer over the status rules', () => {
    const resolved = resolveHwBonus(
      {
        rules: [{ currency: 'TRY', min: 1000, amount: 120 }],
        validityDays: null,
      },
      [{ currency: 'USD', min: 5, amount: 2 }]
    )

    expect(resolved.rules).toEqual([{ currency: 'USD', min: 5, amount: 2 }])
    expect(resolved.validityDays).toBe(FALLBACK_VALIDITY_DAYS)
  })

  it('keeps an empty rule list from the API empty: the bonus is switched off', () => {
    useHwBonusStore.setState({ status: { rules: [], validityDays: null } })
    const { result } = renderHook(() => useHwBonusRules())

    expect(result.current.rules).toEqual([])
  })
})

describe('texts hold no amounts', () => {
  const keys = [
    'bonusExplainTitle',
    'bonusExplainLead',
    'bonusRuleLine',
    'bonusValidityNote',
    'benefitExpiryTitle',
    'benefitExpiryBody',
    'faq3A',
  ] as const

  it.each(['tr', 'en'] as const)(
    '%s texts carry placeholders, not numbers',
    (language) => {
      for (const key of keys) {
        const text = hwTexts[language][key]
        // faq3A also names the 14-day trial credit, which is not a bonus rule.
        const withoutTrial = key === 'faq3A' ? text.replace('14', '') : text
        expect(withoutTrial, `${language}.${key}`).not.toMatch(/\d/)
      }
      expect(hwTexts[language].bonusRuleLine).toContain('{min}')
      expect(hwTexts[language].bonusRuleLine).toContain('{amount}')
      expect(hwTexts[language].bonusValidityNote).toContain('{validityDays}')
    }
  )
})

describe('BonusExplainer', () => {
  it('shows the amounts and the days it got from the API, in Turkish', () => {
    useHwBonusStore.setState({
      status: {
        rules: [{ currency: 'TRY', min: 750, amount: 80 }],
        validityDays: 45,
      },
    })
    render(<BonusExplainer />)

    expect(
      screen.getByRole('heading', {
        name: 'Bakiye yüklediğinizde bonus kazanırsınız',
      })
    ).toBeInTheDocument()
    const item = screen.getByRole('listitem')
    expect(item.textContent).toMatch(/750/)
    expect(item.textContent).toMatch(/80/)
    expect(
      screen.getByText('Bakiyeniz son yüklemeden itibaren 45 gün geçerlidir.')
    ).toBeInTheDocument()
  })

  it('lists the fallback rules and 30 days without API data', () => {
    // Rules only show for the currencies on sale.
    useHwCurrencyStore.setState({
      config: {
        ...HW_LEGACY_CURRENCY_CONFIG,
        currencies: ['TRY', 'EUR', 'USD'],
      },
    })
    render(<BonusExplainer />)

    expect(screen.getAllByRole('listitem')).toHaveLength(
      FALLBACK_BONUS_RULES.length
    )
    expect(
      screen.getByText('Bakiyeniz son yüklemeden itibaren 30 gün geçerlidir.')
    ).toBeInTheDocument()
  })

  it('shows only the validity when the bonus is switched off', () => {
    useHwBonusStore.setState({ status: { rules: [], validityDays: 30 } })
    render(<BonusExplainer />)

    expect(screen.queryByRole('list')).not.toBeInTheDocument()
    expect(screen.queryByRole('heading')).not.toBeInTheDocument()
    expect(screen.getByText(/30 gün geçerlidir/)).toBeInTheDocument()
  })
})
