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

/** More matches than this means the text is too common to change blindly. */
export const MAX_TEXT_MATCHES = 5

/**
 * The interface texts are keyed by their English source text. The admin
 * reports the text as it is displayed, so look that up in the loaded bundle:
 * every key whose current text in lang equals what the admin sees (or whose
 * key is that text, for an untranslated one).
 */
export function findTextKeys(
  bundle: Record<string, unknown> | undefined,
  shown: string
): string[] {
  const wanted = shown.trim()
  if (!bundle || !wanted) return []
  const keys: string[] = []
  for (const [key, value] of Object.entries(bundle)) {
    if (typeof value === 'string' && value.trim() === wanted) keys.push(key)
  }
  if (keys.length === 0 && wanted in bundle) keys.push(wanted)
  return keys
}
