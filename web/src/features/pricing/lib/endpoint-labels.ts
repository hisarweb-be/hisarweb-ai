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

export type EndpointDescription = {
  /** Short plain-language name, e.g. "OpenAI-compatible (Cursor, Cline…)". */
  label: string
  /** One sentence for the hover hint. */
  hint: string
}

/**
 * Plain-language description of an endpoint type. Only the two shapes this
 * site really serves are named: `/v1/chat/completions` (OpenAI shape, used by
 * Cursor, Cline and most coding tools) and `/v1/messages` (Anthropic shape,
 * used by Claude Code). Any other type keeps its technical name.
 */
export function describeEndpoint(
  endpoint: string,
  t: TFunction
): EndpointDescription {
  switch (endpoint) {
    case 'openai':
      return {
        label: t('OpenAI-compatible (Cursor, Cline…)'),
        hint: t(
          'Works with tools that speak the OpenAI format, such as Cursor and Cline. You only change the address and the key in the tool.'
        ),
      }
    case 'anthropic':
      return {
        label: t('Anthropic-compatible (Claude Code)'),
        hint: t(
          'Works with tools that speak the Anthropic format, such as Claude Code. You only change the address and the key in the tool.'
        ),
      }
    default:
      return {
        label: endpoint,
        hint: t('Connection type: {{name}}', { name: endpoint }),
      }
  }
}
