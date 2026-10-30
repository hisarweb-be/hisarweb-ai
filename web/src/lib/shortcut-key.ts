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

/** The letter of the search shortcut. Both key handlers listen for this key. */
export const SEARCH_SHORTCUT_KEY = 'K'

interface NavigatorWithUserAgentData extends Navigator {
  userAgentData?: { platform?: string }
}

/**
 * True on Apple devices, where the shortcut modifier is the Command key.
 * Everywhere else (Windows, Linux, Android) it is Ctrl. The key handlers accept
 * both (`metaKey || ctrlKey`); only the hint shown to the user differs.
 */
export function isApplePlatform(): boolean {
  if (typeof navigator === 'undefined') return false
  const platform =
    (navigator as NavigatorWithUserAgentData).userAgentData?.platform ||
    navigator.platform ||
    navigator.userAgent ||
    ''
  return /mac|iphone|ipad|ipod/i.test(platform)
}

/** The modifier key as printed on the user's keyboard: ⌘ on Apple, Ctrl elsewhere. */
export function modifierKeyLabel(): string {
  return isApplePlatform() ? '⌘' : 'Ctrl'
}

/** The whole hint: ⌘K on Apple devices, Ctrl K elsewhere. */
export function searchShortcutHint(): string {
  const separator = isApplePlatform() ? '' : ' '
  return `${modifierKeyLabel()}${separator}${SEARCH_SHORTCUT_KEY}`
}
