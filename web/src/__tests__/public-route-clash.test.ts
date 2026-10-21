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
import { readdirSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

// A folder in public/ with the name of a page route makes the Go static
// server answer that route with a redirect to the folder, which redirects
// back: /docs looped live after the guide images landed in public/docs.
describe('public folders', () => {
  it('never share a name with a top-level route', () => {
    const root = join(__dirname, '..', '..')
    const routes = new Set(
      readdirSync(join(root, 'src', 'routes')).map((name) =>
        name.replace(/\.tsx$/, '')
      )
    )
    const folders = readdirSync(join(root, 'public'), {
      withFileTypes: true,
    })
      .filter((entry) => entry.isDirectory())
      .map((entry) => entry.name)
    expect(folders.filter((name) => routes.has(name))).toEqual([])
  })
})
