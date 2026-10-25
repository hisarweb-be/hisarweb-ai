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

import { DEFAULT_CONFIG } from '../../../constants'
import type { ModelOption } from '../../../types'
import { getModelFallback } from '../playground-option-utils'

const options = (...values: string[]): ModelOption[] =>
  values.map((value) => ({ label: value, value }))

describe('getModelFallback', () => {
  it('starts on the best-value cheap model', () => {
    expect(DEFAULT_CONFIG.model).toBe('claude-haiku-5-5')
  })

  it('keeps a model that is still offered', () => {
    const models = options('claude-opus-5-5', 'claude-haiku-5-5')
    expect(getModelFallback(models, 'claude-opus-5-5')).toBeNull()
  })

  it('moves a saved older model to its newer successor', () => {
    const models = options(
      'claude-haiku-4-5',
      'claude-haiku-5-5',
      'claude-sonnet-5',
      'claude-sonnet-5-5'
    )
    expect(getModelFallback(models, 'claude-haiku-4-5')).toBe(
      'claude-haiku-5-5'
    )
    expect(getModelFallback(models, 'claude-sonnet-5')).toBe(
      'claude-sonnet-5-5'
    )
  })

  it('keeps an older model when its successor is not offered', () => {
    expect(
      getModelFallback(options('claude-haiku-4-5'), 'claude-haiku-4-5')
    ).toBeNull()
  })

  it('falls back to the best-value model, not the first in the list', () => {
    const models = options(
      'claude-fable-5-1',
      'claude-sonnet-5-5',
      'claude-haiku-5-5'
    )
    expect(getModelFallback(models, 'gpt-4o')).toBe('claude-haiku-5-5')
    expect(
      getModelFallback(
        options('claude-fable-5-1', 'claude-sonnet-5-5'),
        'gpt-4o'
      )
    ).toBe('claude-sonnet-5-5')
  })

  it('uses the first model when no preferred model is offered', () => {
    expect(
      getModelFallback(options('claude-haiku-5-5', 'gemini-3.8-flash'), 'gpt-4o')
    ).toBe('claude-haiku-5-5')
    expect(getModelFallback([], 'gpt-4o')).toBeNull()
  })
})
