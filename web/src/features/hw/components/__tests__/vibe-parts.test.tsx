import { cleanup, render } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'

import { Section } from '../vibe-parts'

afterEach(cleanup)

test('Section renders its title as a level-2 heading', () => {
  const { getByRole } = render(<Section title='Wie het is'>tekst</Section>)

  expect(getByRole('heading', { level: 2, name: 'Wie het is' })).toBeTruthy()
})
