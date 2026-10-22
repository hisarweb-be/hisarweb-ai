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

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

type HintProps = {
  /** The plain-language explanation shown on hover, focus and in the accessible description. */
  hint: string
  children: ReactNode
  className?: string
}

/**
 * A short label with a plain-language explanation on hover or keyboard focus.
 * The catalogue uses it for every technical field, so a beginner can find out
 * what a word means without leaving the page.
 */
export function Hint(props: HintProps) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span
            tabIndex={0}
            aria-description={props.hint}
            className={cn(
              'decoration-muted-foreground/50 cursor-help underline decoration-dotted underline-offset-2',
              props.className
            )}
          />
        }
      >
        {props.children}
      </TooltipTrigger>
      <TooltipContent>{props.hint}</TooltipContent>
    </Tooltip>
  )
}
