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
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { PublicLayout } from '../public-layout'

vi.mock('../footer', () => ({
  Footer: () => <footer data-testid='site-footer' />,
}))

vi.mock('../public-header', () => ({
  PublicHeader: () => <header>nav</header>,
}))

const styles = resolve(import.meta.dirname, '../../../../styles')

test('every public page gets the landing tokens and the aurora backdrop', () => {
  render(
    <PublicLayout>
      <p>page</p>
    </PublicLayout>
  )

  expect(screen.getByTestId('public-layout')).toHaveClass('hwl', 'hwp')
  const backdrop = screen.getByTestId('public-backdrop')
  expect(backdrop).toHaveAttribute('aria-hidden', 'true')
  expect(backdrop.querySelectorAll('.hwl-blob')).toHaveLength(3)
  // The default container starts below the 64px fixed nav, with room to spare.
  expect(screen.getByText('page').closest('main')).toHaveClass('pt-24')
  expect(screen.getByTestId('site-footer')).toBeInTheDocument()
})

test('the landing page keeps its own hero and opts out of the backdrop', () => {
  render(
    <PublicLayout backdrop={false} showMainContainer={false}>
      <p>landing</p>
    </PublicLayout>
  )

  expect(screen.getByTestId('public-layout')).toHaveClass('hwp')
  expect(screen.queryByTestId('public-backdrop')).not.toBeInTheDocument()
})

test('a page without the default container still sits in the one main landmark', () => {
  render(
    <PublicLayout showMainContainer={false}>
      <header>page head</header>
    </PublicLayout>
  )

  // axe: landmark-one-main, and a page header outside main is a second banner.
  expect(screen.getByRole('main')).toContainElement(
    screen.getByText('page head')
  )
})

test('the best price label keeps dark text on green in both themes', () => {
  const css = readFileSync(resolve(styles, 'hw-cinematic.css'), 'utf8')

  // White on the light-theme green measured 3.4:1 (axe color-contrast, WCAG AA needs 4.5).
  expect(css).toMatch(
    /\n\.hwp-best-label \{[^}]*\n {2}color: oklch\(0\.18 0\.03 155\);/
  )
  expect(css).not.toMatch(/\.dark \.hwp-best-label/)
})

test('the landing chips sit above the code window', () => {
  const css = readFileSync(resolve(styles, 'hw-landing.css'), 'utf8')
  const rule = (selector: string) =>
    css.match(new RegExp(`\\n${selector} \\{([^}]*)\\}`))?.[1] ?? ''

  expect(rule('\\.hwl-chip')).toMatch(/z-index: 2;/)
  expect(rule('\\.hwl-stage-card')).toMatch(/z-index: 1;/)
})

test('the cinematic layer stands still under prefers-reduced-motion', () => {
  const css = readFileSync(resolve(styles, 'hw-cinematic.css'), 'utf8')

  expect(css).toMatch(/@media \(prefers-reduced-motion: no-preference\)/)
  expect(css).toMatch(/@media \(prefers-reduced-motion: reduce\)/)
})
