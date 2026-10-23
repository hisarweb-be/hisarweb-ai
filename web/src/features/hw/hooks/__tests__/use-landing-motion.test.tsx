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
import { render } from '@testing-library/react'
import { useRef } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { useLandingMotion } from '../use-landing-motion'

function Probe() {
  const root = useRef<HTMLElement>(null)
  useLandingMotion(root)
  return (
    <main ref={root}>
      <p data-reveal>one</p>
    </main>
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

test('shows every reveal element at once when IntersectionObserver is missing', () => {
  vi.stubGlobal('IntersectionObserver', undefined)
  const view = render(<Probe />)

  expect(view.container.querySelector('p')).toHaveAttribute('data-in')
})

test('keeps a reveal element hidden until it scrolls into view', () => {
  let notify: (entries: unknown[]) => void = () => {}
  class FakeObserver {
    constructor(callback: (entries: unknown[]) => void) {
      notify = callback
    }
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  vi.stubGlobal('IntersectionObserver', FakeObserver)
  const view = render(<Probe />)
  const paragraph = view.container.querySelector('p') as HTMLElement

  expect(paragraph).not.toHaveAttribute('data-in')
  notify([{ isIntersecting: true, target: paragraph }])
  expect(paragraph).toHaveAttribute('data-in')
})
