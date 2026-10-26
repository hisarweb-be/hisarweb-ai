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
import { act, cleanup, render, screen, within } from '@testing-library/react'
import i18next from 'i18next'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { hwTexts } from '../../texts'
import { TrustBlock } from '../trust-block'

async function useLanguage(language: string) {
  i18next.addResourceBundle(language, 'translation', {})
  await act(() => i18next.changeLanguage(language))
}

beforeEach(() => useLanguage('tr'))
afterEach(cleanup)

describe('TrustBlock', () => {
  it('names both payment processors (Revolut for TRY, Stripe for EUR and USD) and that the fees are covered', () => {
    render(<TrustBlock />)

    expect(
      screen.getByText('Ödemeler Revolut ve Stripe ile güvenli şekilde işlenir')
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'Tüm işlem ücretlerini biz karşılıyoruz. Sadece kullandığınız kadar ödersiniz.'
      )
    ).toBeInTheDocument()
  })

  it('shows the six brands: five local logos with Turkish alt texts and a 3D Secure label', () => {
    render(<TrustBlock />)

    const list = screen.getByRole('list')
    const logos = within(list).getAllByRole('img')
    expect(logos.map((logo) => logo.getAttribute('alt'))).toEqual([
      'Stripe logosu',
      'Visa logosu',
      'Mastercard logosu',
      'Troy logosu',
      'American Express logosu',
    ])
    // Local files only: no hotlinks to another host.
    for (const logo of logos) {
      expect(logo.getAttribute('src')).toMatch(/^\/payment-logos\/[a-z]+\.svg$/)
    }
    expect(within(list).getByText('3D Secure')).toBeInTheDocument()
    expect(within(list).getAllByRole('listitem')).toHaveLength(6)
  })

  it('uses the English sentences when the page language is English', async () => {
    await useLanguage('en')
    render(<TrustBlock />)

    expect(
      screen.getByText('Payments are processed securely by Revolut and Stripe')
    ).toBeInTheDocument()
    expect(screen.getByText(hwTexts.en.trustFees)).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Stripe logo' })).toBeInTheDocument()
  })

  it('names itself for assistive technology', () => {
    render(<TrustBlock centered className='mt-4' />)

    expect(
      screen.getByRole('region', { name: hwTexts.tr.trustLogosLabel })
    ).toBeInTheDocument()
  })
})
