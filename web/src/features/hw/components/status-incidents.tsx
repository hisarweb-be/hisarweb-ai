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
import { TriangleAlert } from 'lucide-react'

import {
  formatHwDuration,
  formatHwStamp,
  hwStatusMessage,
} from '../lib/format'
import type { HwLanguage, HwTextKey, HwTextVars } from '../texts'
import type { HwStatusIncident } from '../types'

type HwText = (key: HwTextKey, vars?: HwTextVars) => string

/**
 * The running outage first, then the outages that ended. The length of a
 * running outage is counted from its start, because the server only fills
 * `duration` once an outage has ended.
 */
export function StatusIncidents(props: {
  incident?: HwStatusIncident | null
  recent: HwStatusIncident[]
  language: HwLanguage
  text: HwText
}) {
  const nowSeconds = Math.floor(Date.now() / 1000)

  return (
    <div className='space-y-4'>
      {props.incident != null && (
        <div className='border-destructive/40 bg-destructive/5 rounded-lg border p-4'>
          <p className='text-destructive flex items-center gap-2 text-sm font-semibold'>
            <TriangleAlert className='size-4' aria-hidden='true' />
            {props.text('statusOngoingIncident')}
          </p>
          <p className='mt-2 text-sm'>
            {hwStatusMessage(props.incident, props.language)}
          </p>
          <p className='text-muted-foreground mt-2 text-sm'>
            {props.text('statusIncidentStart', {
              time: formatHwStamp(props.incident.started_at),
            })}
            {' · '}
            {props.text('statusIncidentDuration', {
              duration: formatHwDuration(
                Math.max(0, nowSeconds - props.incident.started_at),
                props.text
              ),
            })}
          </p>
        </div>
      )}

      {props.recent.length > 0 && (
        <div className='space-y-3'>
          <h3 className='text-sm font-semibold'>
            {props.text('statusRecentIncidents')}
          </h3>
          <ul className='space-y-3'>
            {props.recent.map((entry) => (
              <li
                key={`${entry.started_at}-${entry.ended_at}`}
                className='border-border/60 rounded-lg border p-4 text-sm'
              >
                <p>{hwStatusMessage(entry, props.language)}</p>
                <p className='text-muted-foreground mt-2'>
                  {props.text('statusIncidentStart', {
                    time: formatHwStamp(entry.started_at),
                  })}
                  {' · '}
                  {props.text('statusIncidentEnd', {
                    time: formatHwStamp(entry.ended_at),
                  })}
                  {' · '}
                  {props.text('statusIncidentDuration', {
                    duration: formatHwDuration(entry.duration, props.text),
                  })}
                </p>
              </li>
            ))}
          </ul>
        </div>
      )}

      {props.incident == null && props.recent.length === 0 && (
        <p className='text-muted-foreground text-sm'>
          {props.text('statusNoIncidents')}
        </p>
      )}
    </div>
  )
}
