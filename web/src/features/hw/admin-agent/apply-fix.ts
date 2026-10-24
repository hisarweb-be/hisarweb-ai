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
import { applyTextOverride, type AgentTextFix } from './api'
import { findTextKeys, MAX_TEXT_MATCHES } from './find-keys'

export type AppliedFix = { lang: string; keys: string[] }

export type FixTarget = {
  getResourceBundle: (lang: string, ns: string) => Record<string, unknown>
  addResources: (
    lang: string,
    ns: string,
    resources: Record<string, string>
  ) => void
  changeLanguage: (lang: string) => Promise<unknown>
}

/** Applies a proposed text and shows it at once, without a reload. */
export async function applyFix(
  target: FixTarget,
  fix: AgentTextFix
): Promise<AppliedFix | 'not-found' | 'too-many'> {
  const keys = findTextKeys(
    target.getResourceBundle(fix.lang, 'translation'),
    fix.shown
  )
  if (keys.length === 0) {
    return 'not-found'
  }
  if (keys.length > MAX_TEXT_MATCHES) {
    return 'too-many'
  }
  for (const key of keys) {
    await applyTextOverride(fix.lang, key, fix.proposed)
  }
  target.addResources(
    fix.lang,
    'translation',
    Object.fromEntries(keys.map((key) => [key, fix.proposed]))
  )
  await target.changeLanguage(fix.lang)
  return { lang: fix.lang, keys }
}
