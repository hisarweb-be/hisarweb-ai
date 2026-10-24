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

import {
  computeTimeRange,
  formatCalendarDate,
  formatChartTime,
  formatDate,
  formatDateObject,
  formatDateTimeObject,
  formatTimeObject,
  getEndOfDay,
  getIstanbulParts,
  getStartOfDay,
  toStartOfDay,
} from '../time'

// 2026-10-23T21:30:00Z is already 24 October 00:30 in Istanbul (UTC+3).
const BOUNDARY = new Date('2026-10-23T21:30:00Z')

describe('Istanbul date and time notation', () => {
  test('puts an instant just before midnight UTC on the next Istanbul day', () => {
    expect(formatDateObject(BOUNDARY)).toBe('24.10.2026')
    expect(formatTimeObject(BOUNDARY)).toBe('00:30:00')
    expect(formatDateTimeObject(BOUNDARY)).toBe('24.10.2026 00:30:00')
    expect(formatDateTimeObject(BOUNDARY, false)).toBe('24.10.2026 00:30')
  })

  test('uses the 24-hour clock and never AM/PM', () => {
    const afternoon = new Date('2026-10-24T13:05:09Z')
    expect(formatTimeObject(afternoon)).toBe('16:05:09')
    expect(formatDateTimeObject(afternoon)).not.toMatch(/AM|PM/i)
  })

  test('formats Unix seconds as dd.MM.yyyy', () => {
    expect(formatDate(BOUNDARY.getTime() / 1000)).toBe('24.10.2026')
  })

  test('shows a dash for an invalid date', () => {
    expect(formatDateObject(new Date('nope'))).toBe('-')
    expect(formatDateTimeObject(new Date('nope'))).toBe('-')
  })

  test('formats a picked calendar day without a time zone shift', () => {
    expect(formatCalendarDate(new Date(2026, 9, 11))).toBe('11.10.2026')
  })

  test('reads the Istanbul wall clock parts', () => {
    expect(getIstanbulParts(BOUNDARY)).toEqual({
      year: 2026,
      month: 10,
      day: 24,
      hour: 0,
      minute: 30,
      second: 0,
    })
  })
})

describe('Istanbul day boundaries', () => {
  test('starts the day at 00:00 Istanbul time', () => {
    expect(getStartOfDay(BOUNDARY).toISOString()).toBe(
      '2026-10-23T21:00:00.000Z'
    )
    expect(toStartOfDay(BOUNDARY.getTime() / 1000)).toBe(
      Date.parse('2026-10-23T21:00:00Z') / 1000
    )
  })

  test('ends the day at 23:59:59.999 Istanbul time', () => {
    expect(getEndOfDay(BOUNDARY).toISOString()).toBe('2026-10-24T20:59:59.999Z')
  })

  test('builds a day range from Istanbul midnight to Istanbul 23:59:59', () => {
    const range = computeTimeRange(7, BOUNDARY, BOUNDARY, true)
    const start = Date.parse('2026-10-23T21:00:00Z') / 1000
    expect(range.start_timestamp).toBe(start)
    expect(range.end_timestamp).toBe(start + 24 * 3600 - 1)
  })
})

describe('chart time labels', () => {
  const ts = BOUNDARY.getTime() / 1000

  test('labels days with the full date', () => {
    expect(formatChartTime(ts, 'day')).toBe('24.10.2026')
  })

  test('labels hours with date and 24-hour time', () => {
    expect(formatChartTime(ts, 'hour')).toBe('24.10.2026 00:00')
  })
})
