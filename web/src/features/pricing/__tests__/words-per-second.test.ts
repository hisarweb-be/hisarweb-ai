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

import { wordsPerSecond } from '../lib/plain-metrics'

describe('wordsPerSecond', () => {
  it('turns tokens per second into about three quarters as many words', () => {
    expect(wordsPerSecond(40)).toBe(30)
    expect(wordsPerSecond(42)).toBe(32)
  })

  it('never shows less than one word per second for a real speed', () => {
    expect(wordsPerSecond(0.5)).toBe(1)
  })

  it('returns null when no usable speed was measured', () => {
    expect(wordsPerSecond(0)).toBeNull()
    expect(wordsPerSecond(-3)).toBeNull()
    expect(wordsPerSecond(Number.NaN)).toBeNull()
    expect(wordsPerSecond(Number.POSITIVE_INFINITY)).toBeNull()
  })
})
