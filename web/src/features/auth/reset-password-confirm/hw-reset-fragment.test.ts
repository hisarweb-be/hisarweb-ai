import { describe, expect, it } from 'vitest'

import { hwReadResetParams } from './hw-reset-fragment'

describe('hwReadResetParams', () => {
  it('reads e-mail and token from the fragment', () => {
    expect(hwReadResetParams('#email=a%2Bb%40x.com&token=t1', {})).toEqual({
      email: 'a+b@x.com',
      token: 't1',
    })
  })

  it('falls back to the query string of an older link', () => {
    expect(hwReadResetParams('', { email: 'o@x.com', token: 't2' })).toEqual({
      email: 'o@x.com',
      token: 't2',
    })
  })
})
