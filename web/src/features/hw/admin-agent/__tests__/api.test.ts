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

import { assistantLanguage, customerTurns, type AgentTurn } from '../api'

describe('assistantLanguage', () => {
  it('keeps tr and nl, maps every other language to en', () => {
    expect(assistantLanguage('tr-TR')).toBe('tr')
    expect(assistantLanguage('nl')).toBe('nl')
    expect(assistantLanguage('zh-TW')).toBe('en')
    expect(assistantLanguage(undefined)).toBe('tr')
  })
})

describe('customerTurns', () => {
  it('sends the last 12 turns as text only', () => {
    const turns: AgentTurn[] = Array.from({ length: 15 }, (_, at) => ({
      role: at % 2 === 0 ? 'user' : 'assistant',
      content: `m${at}`,
      attachments: [{ name: 'a', mime: 'text/plain', data: 'x' }],
    }))
    const sent = customerTurns(turns)
    expect(sent).toHaveLength(12)
    expect(sent[0]).toEqual({ role: 'assistant', content: 'm3' })
    expect(sent.at(-1)).toEqual({ role: 'user', content: 'm14' })
  })
})
