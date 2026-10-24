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
import { describe, expect, test } from 'vitest'

import type { QuotaDataItem } from '@/features/dashboard/types'
import type { TimeGranularity } from '@/lib/time'

import { processChartData, processUserChartData } from '../charts'

const cases: Array<{
  granularity: TimeGranularity
  start: string
  interval: number
  labels: string[]
}> = [
  {
    granularity: 'hour',
    start: '2025-12-31T20:00:00+03:00',
    interval: 3600,
    labels: [
      '31.12.2025 20:00',
      '31.12.2025 21:00',
      '31.12.2025 22:00',
      '31.12.2025 23:00',
      '01.01.2026 00:00',
      '01.01.2026 01:00',
      '01.01.2026 02:00',
      '01.01.2026 03:00',
    ],
  },
  {
    granularity: 'day',
    start: '2025-12-27T12:00:00+03:00',
    interval: 86400,
    labels: [
      '27.12.2025',
      '28.12.2025',
      '29.12.2025',
      '30.12.2025',
      '31.12.2025',
      '01.01.2026',
      '02.01.2026',
      '03.01.2026',
    ],
  },
  {
    granularity: 'week',
    start: '2025-11-29T12:00:00+03:00',
    interval: 604800,
    labels: [
      '29.11.2025 - 05.12.2025',
      '06.12.2025 - 12.12.2025',
      '13.12.2025 - 19.12.2025',
      '20.12.2025 - 26.12.2025',
      '27.12.2025 - 02.01.2026',
      '03.01.2026 - 09.01.2026',
      '10.01.2026 - 16.01.2026',
      '17.01.2026 - 23.01.2026',
    ],
  },
]

function unorderedUsage(start: string, interval: number): QuotaDataItem[] {
  const first = new Date(start).getTime() / 1000
  const rows = Array.from({ length: 8 }, (_, i) => ({
    created_at: first + i * interval,
    model_name: 'model-a',
    username: 'alice',
    quota: (i + 1) * 500000,
    count: i + 1,
  }))
  // Same hour, or a second hourly record within the same day/week label.
  const duplicate = {
    ...rows[0],
    created_at: first + (interval === 3600 ? 0 : 3600),
    quota: 5000000,
    count: 10,
  }
  return [
    rows[7],
    rows[2],
    duplicate,
    rows[5],
    rows[0],
    rows[6],
    rows[1],
    rows[4],
    rows[3],
  ]
}

describe.each(cases)('dashboard $granularity chart chronology', (scenario) => {
  test('orders model buckets across New Year while aggregating repeated labels', () => {
    const data = unorderedUsage(scenario.start, scenario.interval)
    const original = structuredClone(data)
    const result = processChartData(data, scenario.granularity)

    for (const key of ['spec_line', 'spec_area', 'spec_model_line'] as const) {
      const values: Array<{ Time: string; rawQuota: number; Count: number }> =
        result[key].data[0].values
      expect(values.map((row) => row.Time)).toEqual(scenario.labels)
      expect(
        values.map((row) =>
          key === 'spec_model_line' ? row.Count : row.rawQuota / 500000
        )
      ).toEqual([11, 2, 3, 4, 5, 6, 7, 8])
    }
    expect(data).toEqual(original)
  })

  test('orders user buckets across New Year while aggregating repeated labels', () => {
    const data = unorderedUsage(scenario.start, scenario.interval)
    const original = structuredClone(data)
    const result = processUserChartData(data, scenario.granularity)
    const values: Array<{ Time: string; rawQuota: number }> =
      result.spec_user_trend.data[0].values

    expect(values.map((row) => row.Time)).toEqual(scenario.labels)
    expect(values.map((row) => row.rawQuota / 500000)).toEqual([
      11, 2, 3, 4, 5, 6, 7, 8,
    ])
    expect(data).toEqual(original)
  })

  test('keeps the existing seven padded model buckets in chronological order', () => {
    const data = [unorderedUsage(scenario.start, scenario.interval)[0]]
    const result = processChartData(data, scenario.granularity)

    for (const key of ['spec_line', 'spec_area', 'spec_model_line'] as const) {
      const values: Array<{ Time: string; rawQuota: number; Count: number }> =
        result[key].data[0].values
      expect(values.map((row) => row.Time)).toEqual(scenario.labels.slice(1))
      expect(
        values.map((row) =>
          key === 'spec_model_line' ? row.Count : row.rawQuota / 500000
        )
      ).toEqual([0, 0, 0, 0, 0, 0, 8])
    }
  })
})
