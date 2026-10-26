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

import { describeEndpoint } from '../lib/endpoint-labels'

const t = ((key: string, options?: { name?: string }) =>
  options?.name ? key.replace('{{name}}', options.name) : key) as TFunction

describe('describeEndpoint', () => {
  it('names the tools behind the OpenAI and Anthropic formats', () => {
    expect(describeEndpoint('openai', t).label).toBe(
      'OpenAI-compatible (Cursor, Cline, Codex and more)'
    )
    expect(describeEndpoint('anthropic', t).label).toBe(
      'Anthropic-compatible (Claude Code)'
    )
  })

  it('names the Responses format after the tool that uses it', () => {
    expect(describeEndpoint('openai-response', t).label).toBe(
      'OpenAI Responses (Codex)'
    )
  })

  it('gives every endpoint a one-sentence hint', () => {
    for (const endpoint of [
      'openai',
      'anthropic',
      'openai-response',
      'jina-rerank',
    ]) {
      expect(describeEndpoint(endpoint, t).hint.length).toBeGreaterThan(10)
    }
  })

  it('keeps the raw name of a connection type it makes no claim about', () => {
    const described = describeEndpoint('jina-rerank', t)
    expect(described.label).toBe('jina-rerank')
    expect(described.hint).toContain('jina-rerank')
  })
})
