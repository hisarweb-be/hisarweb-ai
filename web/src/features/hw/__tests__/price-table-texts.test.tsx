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
import { renderHook } from '@testing-library/react'
import { createInstance } from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, test } from 'vitest'

import { usePriceTableText } from '../price-table-texts'

const KEYS = [
  'introWhat',
  'introWhy',
  'tableLabel',
  'colOfficial',
  'colVsOfficial',
  'noteContext',
] as const

function turkishTexts() {
  const i18n = createInstance()
  void i18n.init({ lng: 'tr', resources: {} })
  const wrapper = (props: { children: ReactNode }) => (
    <I18nextProvider i18n={i18n}>{props.children}</I18nextProvider>
  )
  const { result } = renderHook(() => usePriceTableText(), { wrapper })
  return KEYS.map((key) => result.current(key)).join(' ')
}

describe('price table texts', () => {
  test('the Turkish texts do not call the provider price "resmi"', () => {
    expect(turkishTexts()).not.toMatch(/resm[iîı]/i)
  })
})
