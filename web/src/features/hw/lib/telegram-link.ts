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

/** The three ways to open one bot deep link. */
export type TelegramLinks = {
  /** `https://t.me/<bot>?start=<token>`, the link the server created. */
  web: string
  /** `tg://resolve?…`, opened by an installed Telegram app. */
  app: string
  /** Telegram Web K, which opens the same bot chat in the browser. */
  browser: string
}

const BOT_NAME = /^[A-Za-z0-9_]{5,64}$/

/**
 * Derives the app and Telegram Web links from the `t.me` deep link the server
 * answers with. A computer without the Telegram app does nothing with a `t.me`
 * link, so the dialog offers Telegram Web as well; it reads the `tgaddr` hash
 * parameter and opens that `tg://` address. Returns null for anything that is
 * not a `t.me` bot link with a start token.
 */
export function telegramLinks(deepLink: string): TelegramLinks | null {
  let url: URL
  try {
    url = new URL(deepLink)
  } catch {
    return null
  }
  const bot = url.pathname.replace(/^\//, '')
  const start = url.searchParams.get('start') ?? ''
  if (url.protocol !== 'https:' || url.hostname !== 't.me') return null
  if (!BOT_NAME.test(bot) || start === '') return null
  const app = `tg://resolve?domain=${bot}&start=${encodeURIComponent(start)}`
  return {
    web: deepLink,
    app,
    browser: `https://web.telegram.org/k/#?tgaddr=${encodeURIComponent(app)}`,
  }
}
