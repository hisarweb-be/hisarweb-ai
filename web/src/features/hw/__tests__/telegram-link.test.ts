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
import { describe, expect, it } from 'vitest'

import { telegramLinks } from '../lib/telegram-link'

describe('telegramLinks', () => {
  it('derives the app and Telegram Web links from the t.me deep link', () => {
    expect(
      telegramLinks('https://t.me/HisarWebAI_bot?start=abc-DEF_123')
    ).toEqual({
      web: 'https://t.me/HisarWebAI_bot?start=abc-DEF_123',
      app: 'tg://resolve?domain=HisarWebAI_bot&start=abc-DEF_123',
      browser:
        'https://web.telegram.org/k/#?tgaddr=tg%3A%2F%2Fresolve%3Fdomain%3DHisarWebAI_bot%26start%3Dabc-DEF_123',
    })
  })

  it.each([
    'not a url',
    'http://t.me/HisarWebAI_bot?start=abc',
    'https://evil.example/HisarWebAI_bot?start=abc',
    'https://t.me/HisarWebAI_bot',
    'https://t.me/bad%20name?start=abc',
  ])('refuses %s', (link) => {
    expect(telegramLinks(link)).toBeNull()
  })
})
