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
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { SearchBar } from '@/features/pricing/components/search-bar'

import {
  isApplePlatform,
  modifierKeyLabel,
  searchShortcutHint,
} from '../shortcut-key'

function setPlatform(platform: string, userAgentDataPlatform?: string) {
  vi.spyOn(navigator, 'platform', 'get').mockReturnValue(platform)
  if (userAgentDataPlatform !== undefined) {
    Object.defineProperty(navigator, 'userAgentData', {
      configurable: true,
      value: { platform: userAgentDataPlatform },
    })
  }
}

afterEach(() => {
  vi.restoreAllMocks()
  Reflect.deleteProperty(navigator, 'userAgentData')
})

describe('search shortcut hint', () => {
  it('shows Ctrl K on Windows and Linux, where there is no Command key', () => {
    setPlatform('Win32')
    expect(isApplePlatform()).toBe(false)
    expect(modifierKeyLabel()).toBe('Ctrl')
    expect(searchShortcutHint()).toBe('Ctrl K')

    setPlatform('Linux x86_64')
    expect(searchShortcutHint()).toBe('Ctrl K')
  })

  it('shows ⌘K on a Mac, an iPhone and an iPad', () => {
    for (const platform of ['MacIntel', 'iPhone', 'iPad']) {
      setPlatform(platform)
      expect(isApplePlatform()).toBe(true)
      expect(searchShortcutHint()).toBe('⌘K')
    }
  })

  it('prefers the platform the browser reports through userAgentData', () => {
    setPlatform('MacIntel', 'Windows')
    expect(searchShortcutHint()).toBe('Ctrl K')
  })

  it('is what the model search box shows next to the field', () => {
    setPlatform('Win32')
    const { unmount } = render(
      <SearchBar value='' onChange={() => {}} onClear={() => {}} />
    )
    expect(screen.getByText('Ctrl K')).toBeInTheDocument()
    expect(screen.queryByText('⌘K')).toBeNull()
    unmount()

    setPlatform('MacIntel')
    render(<SearchBar value='' onChange={() => {}} onClear={() => {}} />)
    expect(screen.getByText('⌘K')).toBeInTheDocument()
  })
})
