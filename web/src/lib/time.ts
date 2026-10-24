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
/**
 * Time utility functions for consistent time handling across the application
 */
import { APP_LOCALE, APP_TIME_ZONE } from '@/lib/format-locale'

/**
 * Time granularity type
 */
export type TimeGranularity = 'hour' | 'day' | 'week'

/**
 * Convert Date object to Unix timestamp (seconds)
 */
export function dateToUnixTimestamp(date: Date): number {
  return Math.floor(date.getTime() / 1000)
}

/**
 * Wall-clock parts of an instant in Istanbul time. `hourCycle: 'h23'` keeps
 * midnight as 00 and never as 24, and the formatter is built once.
 */
const istanbulPartsFormatter = new Intl.DateTimeFormat(APP_LOCALE, {
  timeZone: APP_TIME_ZONE,
  hourCycle: 'h23',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
})

export type IstanbulParts = {
  year: number
  month: number
  day: number
  hour: number
  minute: number
  second: number
}

/** Istanbul wall-clock parts of a Date or of milliseconds since the epoch. */
export function getIstanbulParts(input: Date | number): IstanbulParts {
  const parts: Record<string, number> = {}
  for (const part of istanbulPartsFormatter.formatToParts(input)) {
    if (part.type !== 'literal') parts[part.type] = Number(part.value)
  }
  return {
    year: parts.year,
    month: parts.month,
    day: parts.day,
    hour: parts.hour,
    minute: parts.minute,
    second: parts.second,
  }
}

/**
 * Milliseconds since the epoch of an Istanbul wall-clock moment. Out-of-range
 * parts roll over like `Date.UTC` (day 0 is the last day of the month before).
 * The zone offset is read back from the formatter instead of being hard coded.
 */
export function istanbulWallClockToMs(
  year: number,
  month: number,
  day: number,
  hour = 0,
  minute = 0,
  second = 0,
  ms = 0
): number {
  const target = Date.UTC(year, month - 1, day, hour, minute, second)
  let guess = target
  for (let i = 0; i < 2; i++) {
    const p = getIstanbulParts(guess)
    const shown = Date.UTC(
      p.year,
      p.month - 1,
      p.day,
      p.hour,
      p.minute,
      p.second
    )
    guess += target - shown
  }
  return guess + ms
}

/**
 * Get the Istanbul start of day (00:00:00) for a Unix timestamp (seconds)
 */
export function toStartOfDay(tsSec: number): number {
  const p = getIstanbulParts(tsSec * 1000)
  return Math.floor(istanbulWallClockToMs(p.year, p.month, p.day) / 1000)
}

/**
 * Get the Istanbul start of day of a Date object
 * Returns new Date with the time set to 00:00:00 Istanbul time
 */
export function getStartOfDay(date: Date = new Date()): Date {
  const p = getIstanbulParts(date)
  return new Date(istanbulWallClockToMs(p.year, p.month, p.day))
}

/**
 * Get the Istanbul end of day of a Date object
 * Returns new Date with the time set to 23:59:59.999 Istanbul time
 */
export function getEndOfDay(date: Date = new Date()): Date {
  const p = getIstanbulParts(date)
  return new Date(
    istanbulWallClockToMs(p.year, p.month, p.day, 23, 59, 59, 999)
  )
}

/**
 * Calculate date range with start and end of day normalization
 * @param days Number of days to go back
 * @param fromDate Starting point (defaults to now)
 * @returns Object with normalized start (00:00:00) and end (23:59:59) dates in
 * Istanbul time
 */
export function getNormalizedDateRange(
  days: number,
  fromDate: Date = new Date()
): { start: Date; end: Date } {
  const p = getIstanbulParts(fromDate)
  return {
    start: new Date(istanbulWallClockToMs(p.year, p.month, p.day - days)),
    end: getEndOfDay(fromDate),
  }
}

/**
 * Calculate a rolling date range ending at the current moment.
 * Example: 1 day means the last 24 hours, not yesterday 00:00 to today 23:59.
 */
export function getRollingDateRange(
  days: number,
  fromDate: Date = new Date()
): { start: Date; end: Date } {
  const end = new Date(fromDate)
  const start = new Date(end.getTime() - days * 24 * 60 * 60 * 1000)

  return { start, end }
}

/**
 * Compute time range as Unix timestamps (seconds)
 * @param days Default number of days if no dates provided
 * @param startDate Optional start date
 * @param endDate Optional end date
 * @param useStartOfDay Whether to normalize to start/end of day
 * @returns Object with start_timestamp and end_timestamp in seconds
 */
export function computeTimeRange(
  days: number,
  startDate?: Date,
  endDate?: Date,
  useStartOfDay = false
): { start_timestamp: number; end_timestamp: number } {
  const now = Math.floor(Date.now() / 1000)

  if (useStartOfDay) {
    const defaultEnd = toStartOfDay(now)
    const end = endDate
      ? toStartOfDay(dateToUnixTimestamp(endDate))
      : defaultEnd
    const start = startDate
      ? toStartOfDay(dateToUnixTimestamp(startDate))
      : end - days * 24 * 3600

    return {
      start_timestamp: start,
      end_timestamp: end + 24 * 3600 - 1, // End of day
    }
  }

  // Normal mode without day normalization
  // Add 1 hour buffer to end time (matches legacy frontend behavior)
  // This ensures the current hour's data is fully included
  const end = endDate ? dateToUnixTimestamp(endDate) : now + 3600
  const start = startDate
    ? dateToUnixTimestamp(startDate)
    : end - days * 24 * 3600

  return { start_timestamp: start, end_timestamp: end }
}

function pad2(value: number): string {
  return String(value).padStart(2, '0')
}

function isValidDate(date: Date): boolean {
  return !Number.isNaN(date.getTime())
}

/** Format a Date object to dd.MM.yyyy in Istanbul time */
export function formatDateObject(date: Date): string {
  if (!isValidDate(date)) return '-'
  const p = getIstanbulParts(date)
  return `${pad2(p.day)}.${pad2(p.month)}.${p.year}`
}

/** Format a Date object to HH:mm:ss (or HH:mm) on the 24-hour clock in Istanbul time */
export function formatTimeObject(date: Date, withSeconds = true): string {
  if (!isValidDate(date)) return '-'
  const p = getIstanbulParts(date)
  const time = `${pad2(p.hour)}:${pad2(p.minute)}`
  return withSeconds ? `${time}:${pad2(p.second)}` : time
}

/** Format a Date object to dd.MM.yyyy HH:mm:ss (or HH:mm) in Istanbul time */
export function formatDateTimeObject(date: Date, withSeconds = true): string {
  if (!isValidDate(date)) return '-'
  return `${formatDateObject(date)} ${formatTimeObject(date, withSeconds)}`
}

/** Format Unix timestamp (seconds) to dd.MM.yyyy in Istanbul time */
export function formatDate(tsSec: number): string {
  return formatDateObject(new Date(tsSec * 1000))
}

/**
 * Format a calendar day picked in a date picker to dd.MM.yyyy. A picked day is
 * a local Date at midnight, so its local year, month and day are the day the
 * visitor chose and no time zone conversion applies.
 */
export function formatCalendarDate(date: Date): string {
  if (!isValidDate(date)) return '-'
  return `${pad2(date.getDate())}.${pad2(date.getMonth() + 1)}.${date.getFullYear()}`
}

/**
 * Format a date and time picked in a date-time picker to dd.MM.yyyy HH:mm:ss
 * (or HH:mm). The visitor typed these wall-clock values into a local input, so
 * the local components are shown as chosen and no time zone conversion applies.
 */
export function formatPickedDateTime(date: Date, withSeconds = true): string {
  if (!isValidDate(date)) return '-'
  const time = `${pad2(date.getHours())}:${pad2(date.getMinutes())}`
  return `${formatCalendarDate(date)} ${withSeconds ? `${time}:${pad2(date.getSeconds())}` : time}`
}

/**
 * Format timestamp for chart display based on time granularity
 * @param timestamp Unix timestamp in seconds
 * @param granularity Time granularity: 'hour', 'day', or 'week'
 * @returns Formatted string (dd.MM.yyyy, Istanbul time) suitable for chart axis
 */
export function formatChartTime(
  timestamp: number,
  granularity: TimeGranularity = 'day'
): string {
  const date = new Date(timestamp * 1000)
  let result = formatDateObject(date)

  if (granularity === 'hour') {
    result += ` ${pad2(getIstanbulParts(date).hour)}:00`
  } else if (granularity === 'week') {
    const p = getIstanbulParts(date)
    const weekEnd = new Date(istanbulWallClockToMs(p.year, p.month, p.day + 6))
    result += ` - ${formatDateObject(weekEnd)}`
  }

  return result
}

/**
 * Add time duration to a date
 * @param months Number of months to add
 * @param days Number of days to add
 * @param hours Number of hours to add
 * @param baseDate Base date to add time to (defaults to now)
 * @returns New date with added time, or undefined if all parameters are 0
 */
export function addTimeToDate(
  months: number,
  days: number,
  hours: number,
  baseDate: Date = new Date()
): Date | undefined {
  if (months === 0 && days === 0 && hours === 0) {
    return undefined
  }

  const result = new Date(baseDate)
  result.setMonth(result.getMonth() + months)
  result.setDate(result.getDate() + days)
  result.setHours(result.getHours() + hours)

  return result
}
