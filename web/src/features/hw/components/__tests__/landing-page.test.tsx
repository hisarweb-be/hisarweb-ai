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
  expect(film).toHaveAttribute('preload', 'none')
  expect(film).toHaveAttribute('poster', '/film/hisarweb-ai-lansman.jpg')
  expect(film).toHaveAttribute('src', '/film/hisarweb-ai-lansman.mp4')
  expect(film).not.toHaveAttribute('autoplay')
  expect(film.querySelector('track')).toBeNull()
})

// Every language is its own film with its own voice and on-screen text, so the
// English page must not play the Turkish file and no subtitle track may return.
test('plays the English film with its own poster on the English page', async () => {
  await useLanguage('en')
  renderLanding()

  const film = await screen.findByLabelText('HisarWeb AI launch film')
  expect(film).toHaveAttribute('src', '/film/hisarweb-ai-lansman-en-v1.mp4')
  expect(film).toHaveAttribute('poster', '/film/hisarweb-ai-lansman-en.jpg')
  expect(film).toHaveAttribute('controls')
  expect(film).not.toHaveAttribute('autoplay')
  expect(film.querySelector('track')).toBeNull()
  expect(
    screen.getByText(/English voice-over and English on-screen text/)
  ).toBeInTheDocument()
})

// The film states the price honestly: half of the official API list price,
// VAT (KDV) included, and its length matches the file (54 s).
test('introduces the film with its real length and the KDV dahil price claim', async () => {
  renderLanding()

  expect(
    await screen.findByRole('heading', { name: /54 saniyede izleyin/ })
  ).toBeInTheDocument()
  expect(
    screen.getByText(/resmî API liste fiyatının yarısına.*KDV dahil/)
  ).toBeInTheDocument()
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
  expect(heading.textContent).toContain('Claude API’si')
})

// The live catalog sells Claude and Grok models only. A promise of Gemini on
// the public pages is a promise HisarWeb AI cannot keep.
test.each(['tr', 'en'] as const)(
  'does not promise Gemini models on the landing page (%s)',
  async (language) => {
    await useLanguage(language)
    renderLanding()

    await screen.findByRole('heading', { level: 1 })
    expect(document.body.textContent).not.toMatch(/gemini/i)
  }
)

test.each(['tr', 'en'] as const)(
  'does not promise Gemini models in the HisarWeb texts (%s)',
  (language) => {
    expect(Object.values(hwTexts[language]).join('\n')).not.toMatch(/gemini/i)
  }
)

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

test('explains below the four steps why the phone is verified', async () => {
  renderLanding()

  expect(
    await screen.findByRole('heading', {
      level: 3,
      name: 'Neden telefon doğrulaması?',
    })
  ).toBeInTheDocument()
  expect(screen.getByText(/SMS göndermeyiz, sizi aramayız/)).toBeInTheDocument()
})
