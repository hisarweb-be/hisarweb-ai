import { describe, expect, it } from 'vitest'

import { formatBuildStamp } from '../build-stamp'

describe('formatBuildStamp', () => {
  it('formats a winter build moment in Brussels time (UTC+1)', () => {
    expect(
      formatBuildStamp('v1.0.0-rc.45', new Date('2026-01-05T23:30:00Z'))
    ).toBe('Versie:1.0.0-rc.45:06-01-26:00:30')
  })

  it('formats a summer build moment in Brussels time (UTC+2)', () => {
    expect(formatBuildStamp('1.2.3', new Date('2026-07-14T08:05:00Z'))).toBe(
      'Versie:1.2.3:14-07-26:10:05'
    )
  })

  it('falls back to 0.0.0 when the version is empty', () => {
    expect(formatBuildStamp(' \n', new Date('2026-10-24T12:00:00Z'))).toBe(
      'Versie:0.0.0:24-10-26:14:00'
    )
  })
})
