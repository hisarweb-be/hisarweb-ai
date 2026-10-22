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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ToolGuides } from '../components/docs-tool-guides'
import {
  ANTHROPIC_BASE_URL,
  GUIDE_UI,
  KEY_PLACEHOLDER,
  KEY_TEST_STEPS,
  MODEL_HAIKU,
  MODEL_HAIKU_FREE,
  OPENAI_BASE_URL,
  TOOL_GUIDES,
  TOOL_ORDER,
  type GuideBlock,
  type GuideLang,
  type ToolGuide,
} from '../docs-tool-guides'

const LANGS: readonly GuideLang[] = ['tr', 'en']
const LONG_TEST = 120_000

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

function renderGuides(lang: GuideLang) {
  return render(
    <I18nextProvider i18n={lang === 'tr' ? trI18n : enI18n}>
      <ToolGuides />
    </I18nextProvider>
  )
}

function tabLabel(guide: ToolGuide, lang: GuideLang): string {
  return guide.tabLabel ? guide.tabLabel[lang] : guide.name
}

/** Opens the tab of a program and returns its panel. */
async function openTool(guide: ToolGuide, lang: GuideLang) {
  const list = screen.getByRole('tablist', { name: GUIDE_UI[lang].pickLabel })
  const tab = within(list).getByRole('tab', { name: tabLabel(guide, lang) })
  await userEvent.click(tab)
  expect(tab).toHaveAttribute('aria-selected', 'true')
  const panelId = tab.getAttribute('aria-controls') ?? ''
  const panel = document.querySelector<HTMLElement>(`[id="${panelId}"]`)
  if (!panel) throw new Error(`No panel for ${guide.id}`)
  return panel
}

function guideOf(id: string): ToolGuide {
  const guide = TOOL_GUIDES.find((candidate) => candidate.id === id)
  if (!guide) throw new Error(`No guide ${id}`)
  return guide
}

function allBlocks(): readonly GuideBlock[] {
  const fromGuides = TOOL_GUIDES.flatMap((guide) =>
    guide.variants.flatMap((variant) =>
      variant.steps.flatMap((step) => step.blocks ?? [])
    )
  )
  const fromKeyTest = KEY_TEST_STEPS.flatMap((step) => step.blocks ?? [])
  return [...fromGuides, ...fromKeyTest]
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** Every `{ tr, en }` pair found anywhere in the data. */
function collectPairs(value: unknown, found: Record<string, unknown>[] = []) {
  if (Array.isArray(value)) {
    for (const item of value) collectPairs(item, found)
  } else if (isRecord(value)) {
    if (typeof value.tr === 'string' && typeof value.en === 'string') {
      found.push(value)
    } else {
      for (const item of Object.values(value)) collectPairs(item, found)
    }
  }
  return found
}

function keyPaths(value: unknown, prefix = ''): string[] {
  if (!isRecord(value)) return [prefix]
  return Object.entries(value).flatMap(([key, item]) =>
    keyPaths(item, `${prefix}.${key}`)
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('program guide data', () => {
  it('has one guide per program, in the required tab order', () => {
    expect(TOOL_GUIDES.map((guide) => guide.id)).toEqual([...TOOL_ORDER])
    expect(TOOL_ORDER).toEqual([
      'claude-code',
      'codex',
      'opencode',
      'zed',
      'cline',
      'copilot',
      'kilo',
      'aider',
      'continue',
      'cursor',
      'antigravity',
    ])
  })

  it('has Turkish and English text of the same shape and never an empty one', () => {
    expect(keyPaths(GUIDE_UI.tr)).toEqual(keyPaths(GUIDE_UI.en))
    const pairs = [
      ...collectPairs(TOOL_GUIDES),
      ...collectPairs(KEY_TEST_STEPS),
    ]
    expect(pairs.length).toBeGreaterThan(100)
    for (const pair of pairs) {
      expect(String(pair.tr).trim()).not.toBe('')
      expect(String(pair.en).trim()).not.toBe('')
    }
    for (const text of [
      ...Object.values(GUIDE_UI.tr),
      ...Object.values(GUIDE_UI.en),
    ]) {
      if (typeof text === 'string') expect(text.trim()).not.toBe('')
    }
  })

  it('puts the key placeholder, and never a real-looking key, in the blocks', () => {
    const blocks = allBlocks()
    const configBlocks = blocks.filter((block) => block.kind === 'config')
    expect(configBlocks.length).toBeGreaterThan(10)
    for (const block of configBlocks) {
      expect(block.code).toContain(KEY_PLACEHOLDER)
    }
    for (const block of blocks) {
      expect(block.code).not.toMatch(/sk-[A-Za-z0-9_-]{20,}/)
    }
  })

  it('builds the protocol URLs of the blocks from the two constants', () => {
    for (const guide of TOOL_GUIDES) {
      const blocks = guide.variants.flatMap((variant) =>
        variant.steps.flatMap((step) => step.blocks ?? [])
      )
      for (const block of blocks) {
        if (!block.code.includes('api.hisarweb.be')) continue
        if (guide.protocol === 'anthropic') {
          expect(block.code).toContain(ANTHROPIC_BASE_URL)
          expect(block.code).not.toContain(`${ANTHROPIC_BASE_URL}/v1`)
        } else {
          expect(block.code).toContain(OPENAI_BASE_URL)
        }
      }
    }
    expect(OPENAI_BASE_URL).toBe('https://api.hisarweb.be/v1')
    expect(ANTHROPIC_BASE_URL).toBe('https://api.hisarweb.be')
  })

  it('marks the honest programs and gives them no steps', () => {
    expect(guideOf('antigravity').status).toBe('unsupported')
    expect(guideOf('cursor').status).toBe('needs-paid-plan')
    expect(guideOf('antigravity').variants).toHaveLength(0)
    expect(guideOf('continue').status).toBe('works')
  })
})

describe.each(LANGS)('program guide in %s', (lang) => {
  const ui = GUIDE_UI[lang]

  it(
    'renders every program tab with its heading and a copy button on every block',
    async () => {
      const errors = vi.spyOn(console, 'error').mockImplementation(() => {})
      renderGuides(lang)

      for (const guide of TOOL_GUIDES) {
        const panel = await openTool(guide, lang)
        expect(
          within(panel).getByRole('heading', { level: 5, name: guide.name })
        ).toBeInTheDocument()
        expect(panel).toHaveTextContent(guide.summary[lang])
        expect(panel).toHaveTextContent(guide.testedWith[lang])

        const blocks = panel.querySelectorAll('[data-guide-block]')
        const copyButtons = within(panel).queryAllByRole('button', {
          name: 'Copy code',
        })
        expect(copyButtons).toHaveLength(blocks.length)
        if (guide.status === 'works') {
          expect(blocks.length).toBeGreaterThan(0)
          expect(
            within(panel).getByRole('heading', { name: ui.worksTitle })
          ).toBeInTheDocument()
        }
      }

      const duplicateKeys = errors.mock.calls.filter((call) =>
        String(call[0]).includes('same key')
      )
      expect(duplicateKeys).toHaveLength(0)
    },
    LONG_TEST
  )

  it('shows the protocol table, the haiku-only trial note and the Roo Code note', () => {
    const { container } = renderGuides(lang)

    const table = screen.getByRole('table')
    expect(within(table).getByText(OPENAI_BASE_URL)).toBeInTheDocument()
    expect(within(table).getByText(ANTHROPIC_BASE_URL)).toBeInTheDocument()
    expect(within(table).getByText(ui.protocolNoSlash)).toBeInTheDocument()
    expect(container).toHaveTextContent(MODEL_HAIKU)
    expect(container).toHaveTextContent(MODEL_HAIKU_FREE)
    expect(container).toHaveTextContent(ui.rooNote)
  })

  it('tests the key first with a Windows PowerShell variant and warns about the key', () => {
    const { container } = renderGuides(lang)

    expect(
      screen.getByRole('heading', { name: ui.keyTestTitle })
    ).toBeInTheDocument()
    expect(screen.getByText(ui.keyTestWarning)).toBeInTheDocument()
    expect(screen.getByText(ui.keyTest401)).toBeInTheDocument()
    expect(
      screen.getAllByRole('tab', { name: 'Windows', selected: true }).length
    ).toBeGreaterThan(0)
    expect(container.textContent).toContain('curl.exe')
    expect(container.textContent).toContain('Invoke-RestMethod')
  })

  it('recommends Cline or OpenCode and jumps to the tab', async () => {
    renderGuides(lang)
    const note = screen.getByText(ui.recommendation).closest('[role="note"]')
    if (!(note instanceof HTMLElement)) throw new Error('No recommendation')

    await userEvent.click(
      within(note).getByRole('button', {
        name: GUIDE_UI[lang].tryInstead.replace('{name}', 'OpenCode'),
      })
    )

    expect(screen.getByRole('tab', { name: 'OpenCode' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  })

  it('tells the truth about Cursor and Antigravity and links to free programs', async () => {
    renderGuides(lang)

    for (const id of ['cursor', 'antigravity']) {
      const guide = guideOf(id)
      const panel = await openTool(guide, lang)
      if (guide.status === 'works' || !guide.limit) {
        throw new Error('Expected an honest program')
      }
      expect(panel).toHaveTextContent(ui.limitTitle[guide.status])
      expect(panel).toHaveTextContent(guide.limit[lang])
      if (guide.status === 'unsupported') {
        expect(
          within(panel).queryByRole('heading', { name: ui.worksTitle })
        ).not.toBeInTheDocument()
        expect(within(panel).queryByRole('list')).not.toBeInTheDocument()
      }

      const zed = within(panel).getByRole('button', {
        name: ui.tryInstead.replace('{name}', 'Zed'),
      })
      within(panel).getByRole('button', {
        name: ui.tryInstead.replace('{name}', 'OpenCode'),
      })
      await userEvent.click(zed)
      expect(screen.getByRole('tab', { name: 'Zed' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(
        screen.getByRole('heading', { level: 5, name: 'Zed' })
      ).toBeInTheDocument()
    }
  })

  it('marks steps that were not tried on our own machine', async () => {
    renderGuides(lang)

    const panel = await openTool(guideOf('cursor'), lang)

    expect(within(panel).getAllByText(ui.unverified).length).toBeGreaterThan(0)
  })

  it(
    'uses no line-clamp or truncate and no three-dot ellipsis character',
    async () => {
      const { container } = renderGuides(lang)

      for (const guide of TOOL_GUIDES) {
        await openTool(guide, lang)
      }

      expect(container.querySelector('[class*="line-clamp"]')).toBeNull()
      expect(container.querySelector('.truncate')).toBeNull()
      expect(container.textContent).not.toContain(String.fromCodePoint(0x2026))
    },
    LONG_TEST
  )

  it('lets the program tabs wrap instead of cutting the titles', () => {
    renderGuides(lang)

    const list = screen.getByRole('tablist', { name: ui.pickLabel })
    expect(list).toHaveClass('flex-wrap')
    const tab = within(list).getByRole('tab', { name: 'Claude Code' })
    expect(tab).toHaveClass('whitespace-normal')
    expect(tab).toHaveClass('max-w-full')
  })
})

describe('program guide content', () => {
  it('opens Codex on Windows with the PowerShell command', async () => {
    renderGuides('en')

    const panel = await openTool(guideOf('codex'), 'en')

    expect(
      within(panel).getByRole('tab', { name: 'Windows', selected: true })
    ).toBeInTheDocument()
    expect(panel.textContent).toContain(OPENAI_BASE_URL)
    expect(panel.textContent).toContain(KEY_PLACEHOLDER)
  })

  it('gives OpenCode both the v1 and the v2 variant', async () => {
    renderGuides('en')

    const panel = await openTool(guideOf('opencode'), 'en')
    const variants = guideOf('opencode').variants

    expect(variants.map((variant) => variant.id)).toEqual(['v1', 'v2'])
    for (const variant of variants) {
      expect(
        within(panel).getByRole('tab', { name: variant.label.en })
      ).toBeInTheDocument()
    }
  })

  it('keeps the Claude Code address without a trailing /v1', async () => {
    renderGuides('tr')

    const panel = await openTool(guideOf('claude-code'), 'tr')

    expect(panel.textContent).toContain(
      `"ANTHROPIC_BASE_URL": "${ANTHROPIC_BASE_URL}"`
    )
    expect(panel.textContent).not.toContain(`${ANTHROPIC_BASE_URL}/v1"`)
  })
})
