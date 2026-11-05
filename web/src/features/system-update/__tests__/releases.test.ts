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

import { syncStatusToRelease } from '../hw-sync-status'
import { compareSystemVersions, getSystemReleaseUrl } from '../releases'

describe('system release ordering', () => {
  test.each([
    ['v0.13.2', 'v1.0.0-alpha.1', -1],
    ['v1.0.0-alpha.2', 'v1.0.0-beta.1', -1],
    ['v1.0.0-beta.1', 'v1.0.0-rc.1', -1],
    ['v1.0.0-rc.9', 'v1.0.0-rc.10', -1],
    ['v1.0.0-rc.19', 'v1.0.0-rc.19-i18nfix.1', -1],
    ['v1.0.0-rc.19-i18nfix.2', 'v1.0.0-rc.20', -1],
    ['v1.0.0-rc.36', 'v1.0.0', -1],
    ['v0.13.1', 'v0.13.1-patch.1', -1],
    ['v0.13.1-patch.9', 'v0.13.1-patch.10', -1],
    ['v0.13.1-patch.10', 'v0.13.2', -1],
    ['v0.8.8.3.2', 'v0.8.8.3.3', -1],
    ['v0.9.3.0', 'v0.9.3', 0],
    ['v1.0.0+build.1', '1.0.0+build.2', 0],
    ['v1.0.0-rc.37', 'v1.0.0-rc.36', 1],
    ['v1.0.0', 'v1.0.0', 0],
    ['v0.0.0', 'v1.0.0', null],
    ['dev', 'v1.0.0', null],
    ['', 'v1.0.0', null],
    ['v1.0.0-custom.1', 'v1.0.0', null],
    ['v1.0.0-rc.36-2-gabcdef', 'v1.0.0', null],
  ])('compares %s against %s as %s', (current, latest, expected) => {
    expect(compareSystemVersions(current, latest)).toBe(expected)
  })
})

describe('sync status payload', () => {
  const status = {
    success: true,
    data: {
      upstreamTag: 'v1.0.0-rc.48',
      mergedTag: 'v1.0.0-rc.47',
      prNumber: 231,
      checkedAt: '2026-11-05T04:21:00Z',
    },
  }

  test('maps the upstream tag and check time onto a release', () => {
    expect(syncStatusToRelease(status)).toEqual({
      tag_name: 'v1.0.0-rc.48',
      published_at: '2026-11-05T04:21:00Z',
      prerelease: false,
      merged: 'v1.0.0-rc.47',
      prNumber: 231,
    })
  })

  test('accepts a status without a merged tag or an open pull request', () => {
    expect(
      syncStatusToRelease({
        success: true,
        data: { ...status.data, mergedTag: null, prNumber: null },
      })
    ).toMatchObject({ merged: null, prNumber: null })
  })

  test('rejects a failed or malformed response instead of reporting that the fork is current', () => {
    expect(() => syncStatusToRelease({ success: false, data: null })).toThrow()
    expect(() => syncStatusToRelease({ message: 'bad response' })).toThrow()
    expect(() =>
      syncStatusToRelease({
        success: true,
        data: { ...status.data, prNumber: '231' },
      })
    ).toThrow()
    expect(() =>
      syncStatusToRelease({
        success: true,
        data: { ...status.data, checkedAt: 'today' },
      })
    ).toThrow()
  })
})

describe('release destination', () => {
  test('points at the open sync pull request when there is one', () => {
    expect(
      getSystemReleaseUrl({
        tag_name: 'v1.0.0-rc.48',
        prerelease: false,
        prNumber: 231,
      })
    ).toBe('https://github.com/hisarweb-be/hisarweb-ai/pull/231')
  })

  test('falls back to our own repository without a pull request', () => {
    for (const prNumber of [null, undefined]) {
      expect(
        getSystemReleaseUrl({
          tag_name: 'v1.0.0-rc.48',
          prerelease: false,
          prNumber,
        })
      ).toBe('https://github.com/hisarweb-be/hisarweb-ai')
    }
  })
})
