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
import { ShieldCheck } from 'lucide-react'

import { cn } from '@/lib/utils'

import { useHwText, type HwTextKey } from '../texts'

interface TrustLogo {
  file: string
  altKey: HwTextKey
}

/**
 * The official marks, stored in `public/payment-logos/` (sources and licences
 * in the README there). 3D Secure has no freely usable mark, so it is a text
 * label below and never a drawn imitation.
 */
const TRUST_LOGOS: readonly TrustLogo[] = [
  { file: 'stripe', altKey: 'trustAltStripe' },
  { file: 'visa', altKey: 'trustAltVisa' },
  { file: 'mastercard', altKey: 'trustAltMastercard' },
  { file: 'troy', altKey: 'trustAltTroy' },
  { file: 'americanexpress', altKey: 'trustAltAmex' },
]

interface TrustBlockProps {
  /** Centre the lines, as in the footer; the payment form keeps them left. */
  centered?: boolean
  className?: string
}

/**
 * Trust block for the footer and the payment form: who processes the payment,
 * which cards and security checks apply, and that HisarWeb pays the fees. The
 * marks sit on a light plate so the dark ones read in both colour schemes.
 */
export function TrustBlock(props: TrustBlockProps) {
  const hw = useHwText()

  return (
    <section
      aria-label={hw('trustLogosLabel')}
      className={cn(
        'flex flex-col gap-3 text-sm',
        props.centered && 'items-center text-center',
        props.className
      )}
    >
      <p
        className={cn(
          'flex items-center gap-2 font-medium',
          props.centered && 'justify-center'
        )}
      >
        <ShieldCheck className='size-4 shrink-0' aria-hidden='true' />
        {hw('trustPayments')}
      </p>
      <ul
        className={cn(
          'flex flex-wrap items-center gap-2',
          props.centered && 'justify-center'
        )}
        role='list'
      >
        {TRUST_LOGOS.map((logo) => (
          <li
            key={logo.file}
            className='flex h-8 items-center rounded-md bg-white px-2 ring-1 ring-black/10'
          >
            <img
              src={`/payment-logos/${logo.file}.svg`}
              alt={hw(logo.altKey)}
              className='h-5 w-auto max-w-14 object-contain'
              loading='lazy'
              decoding='async'
            />
          </li>
        ))}
        <li className='text-muted-foreground flex h-8 items-center rounded-md border px-2 text-xs font-semibold'>
          {hw('trustLabel3ds')}
        </li>
      </ul>
      <p className='text-muted-foreground text-xs'>{hw('trustFees')}</p>
    </section>
  )
}
