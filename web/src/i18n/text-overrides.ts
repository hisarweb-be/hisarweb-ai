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
import type { i18n as I18n } from 'i18next'

/**
 * Texts an admin replaced with the admin agent. They win over the language
 * files, are served by the backend and need no new deploy. A failed request
 * leaves the built-in texts untouched.
 */
export async function loadTextOverrides(
  instance: I18n,
  fetcher: typeof fetch = fetch
): Promise<void> {
  const lang = instance.language || 'tr'
  try {
    const response = await fetcher(
      `/api/hw/text-overrides?lang=${encodeURIComponent(lang)}`
    )
    if (!response.ok) {
      return
    }
    const body = (await response.json()) as {
      success?: boolean
      data?: Record<string, string>
    }
    const overrides = body?.data
    if (!body?.success || !overrides || Object.keys(overrides).length === 0) {
      return
    }
    instance.addResources(lang, 'translation', overrides)
    // Make the mounted components render the new texts.
    await instance.changeLanguage(lang)
  } catch {
    // The built-in texts stay.
  }
}
