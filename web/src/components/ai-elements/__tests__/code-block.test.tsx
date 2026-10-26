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
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import { CodeBlock, CodeBlockEditor } from '../code-block'

afterEach(() => {
  cleanup()
})

function minHeightOf(element: HTMLElement) {
  const host = element.closest<HTMLElement>('[data-slot=code-editor-host]')
  return host?.style.getPropertyValue('--code-editor-min-height')
}

describe('CodeBlock', () => {
  test('is only as tall as a single line of code, with no empty rows below it', () => {
    const { getByRole } = render(
      <CodeBlock code='npm install' language='bash' />
    )

    // One 1.5rem line plus the 2rem of padding, not the four-line minimum.
    expect(minHeightOf(getByRole('textbox', { name: 'bash' }))).toBe('3.5rem')
  })

  test('grows with the number of lines', () => {
    const { getByRole } = render(
      <CodeBlock code={'a\nb\nc\nd\ne\nf'} language='bash' />
    )

    expect(minHeightOf(getByRole('textbox', { name: 'bash' }))).toBe('11rem')
  })
})

describe('CodeBlock accessibility', () => {
  test('names the editable content and lets the scrolling body take keyboard focus', () => {
    const { container } = render(
      <CodeBlock code='npm install' language='bash' />
    )

    expect(
      container.querySelector('.cm-content')?.getAttribute('aria-label')
    ).toBe('bash')
    expect(
      container.querySelector('.code-block-scroll')?.getAttribute('tabindex')
    ).toBe('0')
  })
})

describe('CodeBlockEditor', () => {
  test('keeps room for at least four lines when asked for fewer rows', () => {
    const { getByRole } = render(
      <CodeBlockEditor
        ariaLabel='Short editor'
        autoFocus={false}
        language='markdown'
        onChange={() => undefined}
        rows={1}
        value='hi'
      />
    )

    expect(minHeightOf(getByRole('textbox', { name: 'Short editor' }))).toBe(
      '8rem'
    )
  })
})
