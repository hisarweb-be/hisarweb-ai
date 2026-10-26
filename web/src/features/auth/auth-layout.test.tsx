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
import { expect, test, vi } from 'vitest'

import { AuthLayout } from './auth-layout'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href='/'>{children}</a>
  ),
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({
    systemName: 'Site',
    logo: '/logo.png',
    loading: false,
  }),
}))

vi.mock('@/components/layout/components/footer', () => ({
  Footer: () => <footer data-testid='site-footer' />,
}))

test('the sign-in form and the site name sit in the one main landmark, the footer outside it', () => {
  render(
    <AuthLayout>
      <form aria-label='sign in' />
    </AuthLayout>
  )

  // axe: landmark-one-main and region failed on every auth page.
  const main = screen.getByRole('main')
  expect(main).toContainElement(screen.getByRole('form', { name: 'sign in' }))
  expect(main).toContainElement(screen.getByRole('heading', { name: 'Site' }))
  expect(main).not.toContainElement(screen.getByTestId('site-footer'))
})
