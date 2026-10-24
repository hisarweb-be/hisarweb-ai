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
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'

import { DocsPage } from '../components/docs-page'
import { VibeFilm, VibePage } from '../components/vibe-page'
import { vibeTexts } from '../vibe-texts'

vi.mock('@/components/layout', () => ({
  PublicLayout: (props: { children?: React.ReactNode }) => (
    <main data-testid='public-layout'>{props.children}</main>
  ),
}))
vi.mock('@/components/layout/components/footer', () => ({
  Footer: () => <footer data-testid='site-footer' />,
}))

const trI18n = createInstance()
await trI18n.init({
  lng: 'tr',
  resources: { tr: { translation: {} } },
  initAsync: false,
})

const enI18n = createInstance()
await enI18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

function renderPage(i18n: typeof trI18n) {
  return render(
    <I18nextProvider i18n={i18n}>
      <DocsPage />
    </I18nextProvider>
  )
}

describe('documentation page', () => {
  it('opens the Turkish developer reference with the base URL', async () => {
    renderPage(trI18n)

    await userEvent.click(
      screen.getByRole('button', { name: 'Geliştiriciler için' })
    )

    expect(screen.getByText('API belgeleri')).toBeInTheDocument()
    expect(
      screen.getAllByText('https://api.hisarweb.be/v1').length
    ).toBeGreaterThan(0)
    expect(screen.getByText(/402 yetersiz bakiye/)).toBeInTheDocument()
  })

  it('opens the English developer reference with the base URL', async () => {
    renderPage(enI18n)

    await userEvent.click(
      screen.getByRole('button', { name: 'For developers' })
    )

    expect(screen.getByText('API documentation')).toBeInTheDocument()
    expect(
      screen.getAllByText('https://api.hisarweb.be/v1').length
    ).toBeGreaterThan(0)
    expect(screen.getByText(/402 insufficient balance/)).toBeInTheDocument()
  })

  it('shows the six beginner steps with a screenshot for each visual step', () => {
    renderPage(trI18n)

    expect(screen.getByText('HisarWeb AI nedir?')).toBeInTheDocument()
    for (const title of [
      'Hesap açın',
      '250 TL başlangıç bonusunu alın',
      'API anahtarı oluşturun',
      'Bakiye yükleyin (isteğe bağlı)',
      'Playground’da deneyin',
      'Programınızı seçin',
    ]) {
      expect(screen.getByRole('heading', { name: title })).toBeInTheDocument()
    }
    const images = screen.getAllByRole('img')
    const stepImages = images.filter(
      (image) => !image.getAttribute('src')?.startsWith('/rehber/gids/')
    )
    expect(stepImages).toHaveLength(5)
    const guideImages = images.filter((image) =>
      image.getAttribute('src')?.startsWith('/rehber/gids/claude-code/')
    )
    expect(guideImages.length).toBeGreaterThan(0)
    for (const image of guideImages) {
      expect(image).toHaveAttribute('loading', 'lazy')
      expect(image).toHaveAttribute('width')
      expect(image).toHaveAttribute('height')
    }
    expect(
      screen.getByRole('link', { name: 'Satın al sayfasına git' })
    ).toHaveAttribute('href', '/satin-al')
  })

  it('gives Claude Code the gateway address without a trailing /v1', () => {
    renderPage(trI18n)

    const page = document.body.textContent ?? ''
    expect(page).toContain('"ANTHROPIC_BASE_URL": "https://api.hisarweb.be"')
    expect(page).not.toContain(
      '"ANTHROPIC_BASE_URL": "https://api.hisarweb.be/v1"'
    )
  })

  it('lists the Claude Desktop steps after switching tabs', async () => {
    renderPage(enI18n)

    await userEvent.click(screen.getByRole('tab', { name: 'Desktop app' }))

    expect(
      screen.getByText(/Configure Third-Party Inference/)
    ).toBeInTheDocument()
    expect(screen.getByText(/Save & Restart/)).toBeInTheDocument()
  })

  it('names no supplier anywhere on the page', async () => {
    renderPage(trI18n)
    await userEvent.click(
      screen.getByRole('button', { name: 'Geliştiriciler için' })
    )

    const text = document.body.textContent?.toLowerCase() ?? ''
    expect(text).not.toContain('lumia')
    expect(text).not.toContain('openrouter')
  })
})

function renderVibe(i18n: typeof trI18n) {
  return render(
    <I18nextProvider i18n={i18n}>
      <VibePage />
    </I18nextProvider>
  )
}

describe('documentation page, vibe coding link', () => {
  it('links to the vibe coding guide for beginners', () => {
    renderPage(trI18n)

    expect(
      screen.getByRole('link', {
        name: 'Yeni başlayanlar için vibe coding rehberi',
      })
    ).toHaveAttribute('href', '/vibe-kodlama')
  })
})

describe('vibe coding page', () => {
  it('states the rule in one Turkish sentence and keeps the order of sections', () => {
    renderVibe(trI18n)

    expect(
      screen.getByText('Düşünmeyi abonelik yapar, işi HisarWeb AI yapar.')
    ).toBeInTheDocument()
    const headings = [
      ...document.querySelectorAll('[data-slot="card-title"]'),
    ].map((heading) => heading.textContent)
    expect(headings).toEqual([
      'Kısa film: aboneliğiniz düşünsün, HisarWeb AI işi yapsın',
      'Bu sayfa kimin için?',
      'Kural tek cümle',
      'Tek anahtar, bütün modeller',
      'Önbellek ve token tasarrufu',
      'Araç araç ayarlar',
      'Yönlendirme promptu',
      'HisarWeb AI ne zaman işinize yarar?',
      'Önce ücretsiz deneyin',
    ])
  })

  it('reads English when the interface language is English', () => {
    renderVibe(enI18n)

    expect(
      screen.getByText('The subscription thinks, HisarWeb AI does the work.')
    ).toBeInTheDocument()
  })

  it('warns in a visible box not to switch model inside a session', () => {
    renderVibe(trI18n)

    expect(
      screen.getByText('Önemli: oturumun ortasında model değiştirmeyin')
    ).toBeInTheDocument()
    expect(screen.getByText(/önbellekten \(cache\) yararlanamaz/)).toBeVisible()
  })

  it('prints no subscription price and promises no cache discount', () => {
    renderVibe(trI18n)

    const text = document.body.textContent ?? ''
    expect(text).not.toMatch(/\d\s?(TL|₺)/)
    expect(text).toContain('Önbellek indirimi için bir söz vermiyoruz')
    expect(
      screen.getByRole('link', { name: 'Claude fiyatları (claude.com)' })
    ).toHaveAttribute('href', 'https://claude.com/pricing')
    expect(
      screen.getByRole('link', { name: 'ChatGPT fiyatları (chatgpt.com)' })
    ).toHaveAttribute('href', 'https://chatgpt.com/pricing')
  })

  it('shows copyable settings with the key placeholder and never a real key', () => {
    renderVibe(trI18n)

    const text = document.body.textContent ?? ''
    expect(text).toContain('ANTHROPIC_BASE_URL="https://api.hisarweb.be"')
    expect(text).toContain('CLAUDE_CODE_SUBAGENT_MODEL')
    expect(text).toContain('--architect --model openai/claude-opus-5-5')
    expect(text).toContain('BURAYA_ANAHTARINIZ')
    expect(text).not.toMatch(/sk-[A-Za-z0-9]{10,}/)
    expect(text).toContain('PLAN.md')
  })

  it('says honestly that Cursor needs a paid plan and Roo Code is closed', () => {
    renderVibe(trI18n)

    expect(
      screen.getByText(/ücretli bir Cursor planı ister/)
    ).toBeInTheDocument()
    expect(
      screen.getByText(/Roo Code 15 Mayıs 2026’da kapandı/)
    ).toBeInTheDocument()
  })

  it('links the sign-up and the setup guides', () => {
    renderVibe(trI18n)

    expect(
      screen.getByRole('button', { name: 'Ücretsiz hesap aç' })
    ).toHaveAttribute('href', '/sign-up')
    expect(
      screen.getByRole('link', { name: 'Adım adım kurulum rehberleri' })
    ).toHaveAttribute('href', '/docs')
  })

  it('offers the vibecoder film on the page with controls, poster and no autoplay', () => {
    renderVibe(trI18n)

    expect(
      screen.getByText(
        'Kısa film: aboneliğiniz düşünsün, HisarWeb AI işi yapsın'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByText(/görseller yapay zekâ ile üretilmiştir/)
    ).toBeInTheDocument()
    const film = screen.getByLabelText('HisarWeb AI vibe kodlama tanıtım filmi')
    expect(film).toHaveAttribute('controls')
    expect(film).toHaveAttribute('preload', 'metadata')
    expect(film).toHaveAttribute('poster', '/film/hisarweb-ai-vibe-kodlama.jpg')
    expect(film).toHaveAttribute('src', '/film/hisarweb-ai-vibe-kodlama-v2.mp4')
    expect(film).not.toHaveAttribute('autoplay')
  })

  it('offers Turkish and English subtitles, Turkish on by default on the Turkish page', () => {
    const { container } = renderVibe(trI18n)

    const tracks = container.querySelectorAll('track')
    expect(tracks).toHaveLength(2)
    const turkish = container.querySelector('track[srclang="tr"]')
    const english = container.querySelector('track[srclang="en"]')
    expect(turkish).toHaveAttribute('kind', 'subtitles')
    expect(turkish).toHaveAttribute('label', 'Türkçe')
    expect(turkish).toHaveAttribute(
      'src',
      '/film/hisarweb-ai-vibe-kodlama.tr.vtt'
    )
    expect(turkish).toHaveAttribute('default')
    expect(english).toHaveAttribute('kind', 'subtitles')
    expect(english).toHaveAttribute('label', 'English')
    expect(english).toHaveAttribute(
      'src',
      '/film/hisarweb-ai-vibe-kodlama.en.vtt'
    )
    expect(english).not.toHaveAttribute('default')
  })

  it('switches the English subtitles on by default on the English page', () => {
    const { container } = renderVibe(enI18n)

    expect(container.querySelector('track[srclang="en"]')).toHaveAttribute(
      'default'
    )
    expect(container.querySelector('track[srclang="tr"]')).not.toHaveAttribute(
      'default'
    )
  })

  it('labels the film in English when the page is English', () => {
    renderVibe(enI18n)

    expect(
      screen.getByLabelText('HisarWeb AI film for new vibecoders')
    ).toBeInTheDocument()
  })

  it('offers the film with controls, a poster and no autoplay once it exists', () => {
    render(
      <VibeFilm
        film={{ src: '/film/a.mp4', poster: '/film/a.jpg' }}
        title='Filmi izleyin'
        label='Film'
      />
    )

    const film = screen.getByLabelText('Film')
    expect(film).toHaveAttribute('controls')
    expect(film).toHaveAttribute('preload', 'metadata')
    expect(film).toHaveAttribute('poster', '/film/a.jpg')
    expect(film).not.toHaveAttribute('autoplay')
  })

  it('keeps both languages complete and free of dashes, emoji and suppliers', () => {
    expect(Object.keys(vibeTexts.en).sort()).toEqual(
      Object.keys(vibeTexts.tr).sort()
    )
    for (const language of ['tr', 'en'] as const) {
      const dictionary = JSON.stringify(vibeTexts[language]).toLowerCase()
      expect(dictionary).not.toContain('—')
      expect(dictionary).not.toMatch(/\p{Extended_Pictographic}/u)
      expect(dictionary).not.toContain('lumia')
      expect(dictionary).not.toContain('openrouter')
    }
  })
})
