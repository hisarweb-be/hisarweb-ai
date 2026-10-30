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
import { act, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { LegalConsent } from '../components/legal-consent'
import { SignIn } from '../sign-in'
import { SignUp } from '../sign-up'

const legalStatus = {
  user_agreement_enabled: true,
  privacy_policy_enabled: true,
}

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  Link: (props: { children?: React.ReactNode; to?: string }) => (
    <a href={props.to}>{props.children}</a>
  ),
  useSearch: () => ({}),
}))
vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: legalStatus }),
}))
vi.mock('../auth-layout', () => ({
  AuthLayout: (props: { children?: React.ReactNode }) => (
    <div>{props.children}</div>
  ),
}))
// The forms own the consent checkbox that the submit button depends on. They
// are replaced by that checkbox alone, so the test sees what the page adds
// around the form.
vi.mock('../sign-in/components/user-auth-form', () => ({
  UserAuthForm: () => (
    <LegalConsent
      status={legalStatus}
      checked={false}
      onCheckedChange={vi.fn()}
    />
  ),
}))
vi.mock('../sign-up/components/sign-up-form', () => ({
  SignUpForm: () => (
    <LegalConsent
      status={legalStatus}
      checked={false}
      onCheckedChange={vi.fn()}
    />
  ),
}))

afterEach(async () => {
  await act(() => i18next.changeLanguage('en'))
})

test('translates both legal link labels in the active language', async () => {
  i18next.addResourceBundle('tr', 'translation', {
    'User Agreement': 'Kullanıcı Sözleşmesi',
    'Privacy Policy': 'Gizlilik Politikası',
  })
  await act(() => i18next.changeLanguage('tr'))

  render(
    <LegalConsent
      status={legalStatus}
      checked={false}
      onCheckedChange={vi.fn()}
    />
  )

  expect(
    screen.getByRole('link', { name: 'Kullanıcı Sözleşmesi' })
  ).toHaveAttribute('href', '/user-agreement')
  expect(
    screen.getByRole('link', { name: 'Gizlilik Politikası' })
  ).toHaveAttribute('href', '/privacy-policy')
})

// The sign-in and sign-up pages ask for the legal agreement once, with the
// checkbox that blocks the submit button. A second "by clicking you agree"
// sentence below the form asked for the same consent again.
test.each([
  ['sign-in', SignIn],
  ['sign-up', SignUp],
] as const)('asks for consent only once on the %s page', (_name, Page) => {
  render(<Page />)

  expect(screen.getAllByRole('link', { name: 'User Agreement' })).toHaveLength(
    1
  )
  expect(screen.getAllByRole('link', { name: 'Privacy Policy' })).toHaveLength(
    1
  )
  expect(screen.getAllByRole('checkbox')).toHaveLength(1)
  expect(document.body.textContent).not.toMatch(/you agree to our/i)
})
