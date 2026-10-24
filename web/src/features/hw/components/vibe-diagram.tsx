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
import { Brain, KeyRound, type LucideIcon } from 'lucide-react'

import type { VibeTextKey } from '../vibe-texts'
import type { VibeTranslate } from './vibe-parts'

const EXPERT_ITEMS: VibeTextKey[] = [
  'diagramExpert1',
  'diagramExpert2',
  'diagramExpert3',
  'diagramExpert4',
]
const WORK_ITEMS: VibeTextKey[] = [
  'diagramWork1',
  'diagramWork2',
  'diagramWork3',
  'diagramWork4',
]

function Layer(props: {
  text: VibeTranslate
  title: VibeTextKey
  items: VibeTextKey[]
  icon: LucideIcon
  accent: string
}) {
  return (
    <div className={`bg-background rounded-lg border-2 p-3 ${props.accent}`}>
      <p className='flex items-center gap-2 text-sm font-semibold'>
        <props.icon className='size-4 shrink-0' aria-hidden='true' />
        {props.text(props.title)}
      </p>
      <ul className='mt-2 flex flex-wrap gap-1.5'>
        {props.items.map((key) => (
          <li
            key={key}
            className='bg-muted text-foreground rounded-md px-2 py-0.5 text-xs'
          >
            {props.text(key)}
          </li>
        ))}
      </ul>
    </div>
  )
}

/**
 * The two-layer rule as a small diagram: one task at the top that splits into
 * the subscription (thinking work) and the HisarWeb AI key (the rest).
 * Built from theme colours, so it reads in light and dark mode.
 */
export function VibeDiagram(props: { text: VibeTranslate }) {
  return (
    <figure
      aria-label={props.text('diagramLabel')}
      className='bg-muted/40 rounded-lg border p-4'
    >
      <div className='flex justify-center'>
        <p className='bg-background rounded-full border px-4 py-1.5 text-sm font-semibold'>
          {props.text('diagramTask')}
        </p>
      </div>
      <svg
        viewBox='0 0 200 24'
        preserveAspectRatio='none'
        className='text-muted-foreground block h-6 w-full'
        aria-hidden='true'
      >
        <path
          d='M100 0 V8 M50 8 H150 M50 8 V24 M150 8 V24'
          fill='none'
          stroke='currentColor'
          strokeWidth='1.5'
          vectorEffect='non-scaling-stroke'
        />
      </svg>
      <div className='grid grid-cols-2 gap-3'>
        <Layer
          text={props.text}
          title='diagramExpert'
          items={EXPERT_ITEMS}
          icon={Brain}
          accent='border-amber-500/60'
        />
        <Layer
          text={props.text}
          title='diagramWork'
          items={WORK_ITEMS}
          icon={KeyRound}
          accent='border-primary/60'
        />
      </div>
    </figure>
  )
}
