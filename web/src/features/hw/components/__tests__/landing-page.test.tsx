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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { hwTexts, pickHwLanguage } from '../../texts'
import { LandingPage } from '../landing-page'

// The page reads its dictionary through the resolved i18next language, so the
// tests switch that language the same way the header's language switcher does.
async function useLanguage(language: string) {
  i18next.addResourceBundle(language, 'translation', {})
  await i18next.changeLanguage(language)
}

function renderLanding() {
  const rootRoute = createRootRoute({ component: Outlet })
  const router = createRouter({
    routeTree: rootRoute.addChildren([
      createRoute({
        getParentRoute: () => rootRoute,
        path: '/',
        component: LandingPage,
      }),
      createRoute({
        getParentRoute: () => rootRoute,
        path: '/sign-up',
        component: () => <div>sign-up</div>,
      }),
    ]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(<RouterProvider router={router} />)
}

beforeEach(async () => {
  await useLanguage('tr')
})

afterEach(async () => {
  await useLanguage('en')
})

test('shows the Turkish headline when the interface language is Turkish', async () => {
  renderLanding()

  expect(
    await screen.findByRole('heading', {
      level: 1,
      name: /resmî fiyatın yarısına/,
    })
  ).toBeInTheDocument()
})

test('shows the English headline when the interface language is English', async () => {
  await useLanguage('en')
  renderLanding()

  expect(
    await screen.findByRole('heading', {
      level: 1,
      name: /at half the official price/,
    })
  ).toBeInTheDocument()
})

// The shared Button renders as an anchor with `role="button"`, so the call to
// action is found by that role and its destination is read from `href`.
test('sends every sign-up call to action to the registration page', async () => {
  renderLanding()

  const actions = await screen.findAllByRole('button', {
    name: 'Ücretsiz hesap aç',
  })
  expect(actions.length).toBeGreaterThan(1)
  for (const action of actions) {
    expect(action).toHaveAttribute('href', '/sign-up')
  }
})

test('links the comparator and quality teasers to their public pages', async () => {
  renderLanding()

  expect(
    await screen.findByRole('button', { name: 'Fiyatları karşılaştır' })
  ).toHaveAttribute('href', '/fiyat-karsilastirma')
  expect(
    screen.getByRole('button', { name: 'Fiyat karşılaştırmasına git' })
  ).toHaveAttribute('href', '/fiyat-karsilastirma')
  expect(
    screen.getByRole('link', { name: 'Kalite ölçümlerini görün' })
  ).toHaveAttribute('href', '/kalite')
})

// The film must never start by itself (it has sound) and must not pull the
// whole file on page load: controls stay visible and only metadata is fetched.
test('offers the launch film with controls, a poster and no autoplay', async () => {
  renderLanding()

  const film = await screen.findByLabelText('HisarWeb AI tanıtım filmi')
  expect(film).toHaveAttribute('controls')
  expect(film).toHaveAttribute('preload', 'metadata')
  expect(film).toHaveAttribute('poster', '/film/hisarweb-ai-lansman.jpg')
  expect(film).not.toHaveAttribute('autoplay')
})

test('announces the free trial and the 30-day balance validity', async () => {
  renderLanding()

  expect(
    await screen.findByRole('heading', { level: 2, name: /ücretsiz deneyin/i })
  ).toBeInTheDocument()
  expect(
    screen.getByRole('heading', { level: 3, name: 'Bakiyeniz 30 gün geçerli' })
  ).toBeInTheDocument()
  expect(
    screen.getByText(/250 TL deneme bakiyesi tanımlanır ve 14 gün/)
  ).toBeInTheDocument()
})

test('never names the upstream suppliers in its visible text or dictionary', async () => {
  renderLanding()

  await screen.findByRole('heading', { level: 1 })
  const rendered = document.body.textContent?.toLowerCase() ?? ''
  const dictionary = JSON.stringify(hwTexts).toLowerCase()

  for (const supplier of ['lumia', 'openrouter']) {
    expect(rendered).not.toContain(supplier)
    expect(dictionary).not.toContain(supplier)
  }
})

test('reads Turkish for any language other than an explicit English choice', () => {
  expect(pickHwLanguage(undefined)).toBe('tr')
  expect(pickHwLanguage('tr')).toBe('tr')
  expect(pickHwLanguage('de')).toBe('tr')
  expect(pickHwLanguage('nl-BE')).toBe('tr')
  expect(pickHwLanguage('en')).toBe('en')
  expect(pickHwLanguage('en-GB')).toBe('en')
})

test('highlights only the accent phrase inside the headline', async () => {
  renderLanding()

  const heading = await screen.findByRole('heading', { level: 1 })
  const accent = heading.querySelector('.hwl-accent')
  expect(accent).toHaveTextContent('resmî fiyatın yarısına')
  expect(heading.textContent).toContain('Claude, GPT ve Gemini')
})

test('teases the vibe coding guide with a button to its page', async () => {
  renderLanding()

  expect(
    await screen.findByRole('heading', {
      level: 2,
      name: 'Yeni başlayanlar için vibe coding düzeni',
    })
  ).toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Vibe coding rehberini oku' })
  ).toHaveAttribute('href', '/vibe-kodlama')
})
