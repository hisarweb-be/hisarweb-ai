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
import type { ReactNode } from 'react'
import { describe, expect, test, vi } from 'vitest'

import { Footer } from '../footer'

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { to: string; children: ReactNode; className?: string }) => (
    <a href={props.to} className={props.className}>
      {props.children}
    </a>
  ),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: { user_agreement_enabled: true, privacy_policy_enabled: true },
  }),
}))

vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({
    systemName: 'HisarWeb AI',
    logo: '',
    footerHtml: '',
    demoSiteEnabled: false,
  }),
}))

vi.mock('@/lib/build-stamp', () => ({
  BUILD_STAMP: 'Versie:1.0.0-rc.45:24-10-26:20:13',
}))

describe('site footer', () => {
  test('full footer shows the brand, legal links and the build stamp', () => {
    render(<Footer />)

    expect(screen.getByText('HisarWeb AI')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'User Agreement' })
    ).toHaveAttribute('href', '/user-agreement')
    expect(screen.getByTestId('build-stamp')).toHaveTextContent(
      'Versie:1.0.0-rc.45:24-10-26:20:13'
    )
  })

  test('the brand logo is decorative because the brand name stands next to it', () => {
    const { container } = render(<Footer />)

    // axe image-redundant-alt: alt text repeating the adjacent text.
    expect(container.querySelector('footer img')).toHaveAttribute('alt', '')
  })

  test('compact footer keeps the licence links clickable without the brand block', () => {
    render(<Footer variant='compact' />)

    expect(screen.queryByText('Powerful API Management Platform')).toBeNull()
    expect(
      screen.getByRole('link', {
        name: 'HisarWeb AI is based on New API (AGPL-3.0) by QuantumNous.',
      })
    ).toHaveAttribute('href', 'https://github.com/QuantumNous/new-api')
    expect(
      screen.getByText(
        'Frontend design and development by New API contributors.'
      )
    ).toBeVisible()
    expect(screen.getByTestId('build-stamp')).toBeInTheDocument()
  })

  // The floating assistant button (size-14 at right-4 bottom-4, 72 px of the
  // bottom edge) covered the last footer lines on a phone. Below 640 px the
  // footer keeps at least that much room under its last line.
  test('both footers leave room below the last line for the floating button on a phone', () => {
    const full = render(<Footer />)
    expect(full.container.querySelector('footer > div')).toHaveClass(
      'pb-24',
      'sm:pb-12'
    )
    full.unmount()

    const compact = render(<Footer variant='compact' />)
    expect(compact.container.querySelector('footer')).toHaveClass(
      'pb-20',
      'sm:pb-3'
    )
  })
})
