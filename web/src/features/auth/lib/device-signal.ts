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

// A stable, privacy-preserving browser signal for the signup abuse check.
//
// The server accepts a hex token of 16 to 128 characters in the X-HW-Device
// header, hashes it again with its own secret and keeps only that hash. This
// helper therefore never sends a raw browser trait: it hashes a small set of
// coarse traits (screen, language, time zone, hardware concurrency) plus a
// canvas rendering into one hex string. The token is only as stable as the
// browser: a private window or a changed screen size yields another token,
// which costs the free trial credit but never blocks the signup.

const HW_DEVICE_TOKEN_KEY = 'hw-device-token'

let cachedToken: string | null = null

function readCachedToken(): string | null {
  if (cachedToken) return cachedToken
  try {
    cachedToken = window.sessionStorage.getItem(HW_DEVICE_TOKEN_KEY)
  } catch {
    cachedToken = null
  }
  return cachedToken
}

function storeToken(token: string): string {
  cachedToken = token
  try {
    window.sessionStorage.setItem(HW_DEVICE_TOKEN_KEY, token)
  } catch {
    // A blocked sessionStorage only costs the cache, not the token.
  }
  return token
}

function browserTraits(): string {
  const navigatorInfo = window.navigator
  const screenInfo = window.screen
  return [
    screenInfo?.width,
    screenInfo?.height,
    screenInfo?.colorDepth,
    navigatorInfo.language,
    navigatorInfo.languages?.join(','),
    navigatorInfo.hardwareConcurrency,
    Intl.DateTimeFormat().resolvedOptions().timeZone,
  ]
    .map((value) => String(value ?? ''))
    .join('|')
}

function canvasTrait(): string {
  try {
    const canvas = document.createElement('canvas')
    const context = canvas.getContext('2d')
    if (!context) return ''
    context.textBaseline = 'top'
    context.font = '14px Arial'
    context.fillStyle = '#f60'
    context.fillRect(0, 0, 60, 20)
    context.fillStyle = '#069'
    context.fillText('HisarWeb', 2, 2)
    return canvas.toDataURL().slice(-32)
  } catch {
    return ''
  }
}

function fallbackHex(value: string): string {
  let hash = 0x811c9dc5
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 0x01000193) >>> 0
  }
  return hash.toString(16).padStart(8, '0').repeat(8)
}

async function hashHex(value: string): Promise<string> {
  const subtle = window.crypto?.subtle
  if (!subtle) return fallbackHex(value)
  const digest = await subtle.digest('SHA-256', new TextEncoder().encode(value))
  return Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, '0')
  ).join('')
}

/** Returns the device token sent as X-HW-Device on registration. */
export async function getHwDeviceToken(): Promise<string> {
  if (typeof window === 'undefined') return ''
  const cached = readCachedToken()
  if (cached) return cached
  return storeToken(await hashHex(`${browserTraits()}|${canvasTrait()}`))
}
