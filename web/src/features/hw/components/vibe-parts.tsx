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
import type { ReactNode } from 'react'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import type { useVibeText, VibeTextKey } from '../vibe-texts'

/** Small building blocks shared by the parts of the `/vibe-kodlama` page. */
export type VibeTranslate = ReturnType<typeof useVibeText>

export function Section(props: { title: string; children?: ReactNode }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className='text-xl'>{props.title}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-3'>{props.children}</CardContent>
    </Card>
  )
}

export function Paragraph(props: { children: ReactNode }) {
  return (
    <p className='text-muted-foreground text-sm leading-relaxed'>
      {props.children}
    </p>
  )
}

export function BulletList(props: {
  text: VibeTranslate
  keys: VibeTextKey[]
}) {
  return (
    <ul className='text-muted-foreground list-disc space-y-1.5 ps-5 text-sm'>
      {props.keys.map((key) => (
        <li key={key}>{props.text(key)}</li>
      ))}
    </ul>
  )
}

/** Every link on this page opens in a new tab, so the reader keeps their place. */
export function NewTabLink(props: {
  href: string
  children: ReactNode
  className?: string
}) {
  return (
    <a
      href={props.href}
      target='_blank'
      rel='noopener noreferrer'
      className={props.className ?? 'text-primary hover:underline'}
    >
      {props.children}
    </a>
  )
}
