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
import { useLayoutEffect, type RefObject } from 'react'

/**
 * Scroll and pointer motion for the landing page, without a library.
 *
 * `.hwl-js` on the root arms the reveal styles; elements marked `data-reveal`
 * (or the steps) get `data-in` the first time they scroll into view. Without
 * JavaScript, or under prefers-reduced-motion, the CSS shows the finished
 * picture instead, so nothing is ever left hidden.
 */
export function useLandingMotion(root: RefObject<HTMLElement | null>) {
  useLayoutEffect(() => {
    const element = root.current
    if (!element) return
    element.classList.add('hwl-js')

    // Without IntersectionObserver (old browsers, jsdom) show everything now.
    if (typeof IntersectionObserver === 'undefined') {
      for (const target of element.querySelectorAll('[data-reveal]')) {
        target.setAttribute('data-in', '')
      }
      return () => element.classList.remove('hwl-js')
    }

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) continue
          entry.target.setAttribute('data-in', '')
          observer.unobserve(entry.target)
        }
      },
      { rootMargin: '0px 0px -12% 0px', threshold: 0.12 }
    )
    for (const target of element.querySelectorAll('[data-reveal]')) {
      observer.observe(target)
    }

    // Cursor-following light on the hero and the cards; skipped on touch.
    const fine =
      typeof window.matchMedia === 'function' &&
      window.matchMedia('(hover: hover) and (pointer: fine)').matches
    let frame = 0
    const onMove = (event: PointerEvent) => {
      if (!fine || frame) return
      frame = requestAnimationFrame(() => {
        frame = 0
        for (const glow of element.querySelectorAll<HTMLElement>(
          '[data-glow]'
        )) {
          const box = glow.getBoundingClientRect()
          glow.style.setProperty('--mx', `${event.clientX - box.left}px`)
          glow.style.setProperty('--my', `${event.clientY - box.top}px`)
        }
      })
    }
    element.addEventListener('pointermove', onMove, { passive: true })

    return () => {
      observer.disconnect()
      element.removeEventListener('pointermove', onMove)
      cancelAnimationFrame(frame)
      element.classList.remove('hwl-js')
    }
  }, [root])
}
