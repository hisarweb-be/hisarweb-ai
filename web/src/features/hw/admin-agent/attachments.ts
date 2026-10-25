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
import type { AgentAttachment } from './api'

export const MAX_ATTACHMENTS = 3
export const MAX_IMAGE_BYTES = 2 * 1024 * 1024
export const MAX_TEXT_CHARS = 12000

const IMAGE_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif'])

export type AttachmentProblem = 'type' | 'size'

function readAs(file: File, mode: 'url' | 'text'): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.addEventListener('load', () => resolve(String(reader.result ?? '')))
    reader.addEventListener('error', () => reject(reader.error))
    if (mode === 'url') reader.readAsDataURL(file)
    else reader.readAsText(file)
  })
}

/** Turns a picked, dropped or pasted file into an attachment, or says why not. */
export async function readAttachment(
  file: File
): Promise<AgentAttachment | AttachmentProblem> {
  const name = file.name || 'screenshot'
  if (IMAGE_TYPES.has(file.type)) {
    if (file.size > MAX_IMAGE_BYTES) return 'size'
    return { name, mime: file.type, data: await readAs(file, 'url') }
  }
  if (file.type.startsWith('text/') || /\.(txt|log|md|json|csv)$/i.test(name)) {
    const data = await readAs(file, 'text')
    if (!data.trim() || data.length > MAX_TEXT_CHARS) return 'size'
    return { name, mime: 'text/plain', data }
  }
  return 'type'
}

/** The files on the clipboard, for a pasted screenshot. */
export function filesFromClipboard(data: DataTransfer | null): File[] {
  if (!data) return []
  return [...(data.items ?? [])]
    .filter((item) => item.kind === 'file')
    .map((item) => item.getAsFile())
    .filter((file): file is File => file !== null)
}
