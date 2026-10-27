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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { loadTextOverrides } from '@/i18n/text-overrides'

import { applyFix } from '../apply-fix'
import { AssistantWidget } from '../assistant-widget'
import { findTextKeys, MAX_TEXT_MATCHES } from '../find-keys'

const admin = vi.hoisted(() => ({ isAdmin: false }))
const api = vi.hoisted(() => ({
  applyTextOverride: vi.fn(),
  fetchAssistantFaq: vi.fn(),
  askAssistantFaq: vi.fn(),
  askAssistantFaqAdmin: vi.fn(),
  askAdminAgent: vi.fn(),
}))

vi.mock('@/hooks/use-admin', () => ({ useIsAdmin: () => admin.isAdmin }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: {},
  }),
}))
vi.mock('../api', async (original) => ({
  ...(await original<typeof import('../api')>()),
  applyTextOverride: api.applyTextOverride,
  fetchAssistantFaq: api.fetchAssistantFaq,
  askAssistantFaq: api.askAssistantFaq,
  askAssistantFaqAdmin: api.askAssistantFaqAdmin,
  askAdminAgent: api.askAdminAgent,
}))

describe('findTextKeys', () => {
  const bundle = { Save: 'Sakla', Store: 'Sakla', Cancel: 'Vazgeç', Plain: 'x' }

  it('finds every key that shows the reported text', () => {
    expect(findTextKeys(bundle, ' Sakla ')).toEqual(['Save', 'Store'])
  })

  it('falls back to the key itself for an untranslated text', () => {
    expect(findTextKeys({ 'Hard word': 'Hard word' }, 'Hard word')).toEqual([
      'Hard word',
    ])
    expect(findTextKeys({ 'Hard word': 'Zor' }, 'Hard word')).toEqual([
      'Hard word',
    ])
  })

  it('returns nothing for an unknown or empty text', () => {
    expect(findTextKeys(bundle, 'Nope')).toEqual([])
    expect(findTextKeys(bundle, '  ')).toEqual([])
    expect(findTextKeys(undefined, 'Sakla')).toEqual([])
  })
})

describe('applyFix', () => {
  beforeEach(() => api.applyTextOverride.mockReset())

  const target = (bundle: Record<string, string>) => ({
    getResourceBundle: vi.fn(() => bundle),
    addResources: vi.fn(),
    changeLanguage: vi.fn(async () => undefined),
  })
  const fix = { lang: 'tr', shown: 'Sakla', proposed: 'Kaydet' }

  it('stores the text for every matching key and shows it at once', async () => {
    const i18n = target({ Save: 'Sakla', Store: 'Sakla' })
    const result = await applyFix(i18n, fix)
    expect(result).toEqual({ lang: 'tr', keys: ['Save', 'Store'] })
    expect(api.applyTextOverride).toHaveBeenCalledWith('tr', 'Save', 'Kaydet')
    expect(api.applyTextOverride).toHaveBeenCalledWith('tr', 'Store', 'Kaydet')
    expect(i18n.addResources).toHaveBeenCalledWith('tr', 'translation', {
      Save: 'Kaydet',
      Store: 'Kaydet',
    })
    expect(i18n.changeLanguage).toHaveBeenCalledWith('tr')
  })

  it('stores nothing when the text is not on the screen', async () => {
    const i18n = target({ Cancel: 'Vazgeç' })
    expect(await applyFix(i18n, fix)).toBe('not-found')
    expect(api.applyTextOverride).not.toHaveBeenCalled()
  })

  it('refuses a text that is used in too many places', async () => {
    const crowded = Object.fromEntries(
      Array.from({ length: MAX_TEXT_MATCHES + 1 }, (_, i) => [`k${i}`, 'Sakla'])
    )
    const i18n = target(crowded)
    expect(await applyFix(i18n, fix)).toBe('too-many')
    expect(api.applyTextOverride).not.toHaveBeenCalled()
    expect(i18n.addResources).not.toHaveBeenCalled()
  })
})

describe('AssistantWidget', () => {
  beforeEach(() => {
    api.fetchAssistantFaq.mockReset()
    api.askAssistantFaq.mockReset()
    api.askAssistantFaqAdmin.mockReset()
    api.askAdminAgent.mockReset()
    api.fetchAssistantFaq.mockResolvedValue([
      { id: 'identity', question: 'Who are you?', answer: 'x' },
    ])
  })

  it('shows the Telegram icon with an online light to a customer', () => {
    admin.isAdmin = false
    render(<AssistantWidget />)
    expect(screen.getByLabelText('Assistant')).toBeTruthy()
    expect(screen.getByTestId('assistant-online')).toBeTruthy()
    expect(screen.queryByLabelText('Admin assistant')).toBeNull()
  })

  it('labels the button for an admin', () => {
    admin.isAdmin = true
    render(<AssistantWidget />)
    expect(screen.getByLabelText('Admin assistant')).toBeTruthy()
  })

  it('answers a fixed question without asking the model', async () => {
    admin.isAdmin = true
    api.askAssistantFaqAdmin.mockResolvedValue({
      matched: true,
      id: 'identity',
      answer: 'I am Altuner, founder of HisarWeb AI.',
    })
    render(<AssistantWidget />)
    fireEvent.click(screen.getByLabelText('Admin assistant'))
    fireEvent.click(await screen.findByText('Who are you?'))
    expect(
      await screen.findByText('I am Altuner, founder of HisarWeb AI.')
    ).toBeTruthy()
    expect(api.askAdminAgent).not.toHaveBeenCalled()
  })

  it('gives a customer no model answer for an unknown question', async () => {
    admin.isAdmin = false
    api.askAssistantFaq.mockResolvedValue({ matched: false })
    render(<AssistantWidget />)
    fireEvent.click(screen.getByLabelText('Assistant'))
    const box = await screen.findByLabelText('Type your question')
    fireEvent.change(box, { target: { value: 'something odd' } })
    fireEvent.click(screen.getByLabelText('Send'))
    await waitFor(() =>
      expect(screen.getByText(/no ready answer/)).toBeTruthy()
    )
    expect(api.askAdminAgent).not.toHaveBeenCalled()
  })

  it('adds a pasted screenshot as an attachment for an admin', async () => {
    admin.isAdmin = true
    render(<AssistantWidget />)
    fireEvent.click(screen.getByLabelText('Admin assistant'))
    const box = await screen.findByLabelText('What is wrong or unclear?')
    const file = new File(['png'], 'shot.png', { type: 'image/png' })
    fireEvent.paste(box, {
      clipboardData: { items: [{ kind: 'file', type: 'image/png', getAsFile: () => file }], files: [file] },
    })
    expect(await screen.findByLabelText('Remove attachment')).toBeTruthy()
  })
})

describe('loadTextOverrides', () => {
  const instance = () => ({
    language: 'tr',
    addResources: vi.fn(),
    changeLanguage: vi.fn(async () => undefined),
  })
  const answer = (body: unknown, ok = true) =>
    vi.fn(async () => ({ ok, json: async () => body }) as Response)

  it('adds the stored texts and refreshes the screen', async () => {
    const i18n = instance()
    const fetcher = answer({ success: true, data: { Save: 'Kaydet' } })
    await loadTextOverrides(i18n as never, fetcher)
    expect(fetcher).toHaveBeenCalledWith('/api/hw/text-overrides?lang=tr')
    expect(i18n.addResources).toHaveBeenCalledWith('tr', 'translation', {
      Save: 'Kaydet',
    })
    expect(i18n.changeLanguage).toHaveBeenCalledWith('tr')
  })

  it('leaves the built-in texts alone when there is nothing or it fails', async () => {
    for (const fetcher of [
      answer({ success: true, data: {} }),
      answer({ success: false }),
      answer({}, false),
      vi.fn(async () => {
        throw new Error('offline')
      }),
    ]) {
      const i18n = instance()
      await loadTextOverrides(i18n as never, fetcher as never)
      expect(i18n.addResources).not.toHaveBeenCalled()
      expect(i18n.changeLanguage).not.toHaveBeenCalled()
    }
  })
})
