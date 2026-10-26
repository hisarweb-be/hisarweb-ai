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
import { Link } from '@tanstack/react-router'
import { Fragment, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { AttributionNotice } from '@/components/attribution-notice'
import { TrustBlock } from '@/features/hw/components/trust-block'
import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
import { BUILD_STAMP } from '@/lib/build-stamp'
import { DEFAULT_SYSTEM_NAME } from '@/lib/constants'
import { cn } from '@/lib/utils'

interface FooterLink {
  text: string
  href: string
}

interface FooterColumnProps {
  title: string
  links: FooterLink[]
}

interface FooterProps {
  logo?: string
  name?: string
  columns?: FooterColumnProps[]
  copyright?: string
  className?: string
  /** compact: one centered line for the dashboard and the sign-in screens. */
  variant?: 'full' | 'compact'
}

function FooterLinkItem(props: { link: FooterLink }) {
  const { t } = useTranslation()
  const isExternal = props.link.href.startsWith('http')
  const label = t(props.link.text)

  if (isExternal) {
    return (
      <a
        href={props.link.href}
        target='_blank'
        rel='noopener noreferrer'
        className='text-muted-foreground hover:text-foreground text-sm transition-colors duration-200'
      >
        {label}
      </a>
    )
  }

  return (
    <Link
      to={props.link.href}
      className='text-muted-foreground hover:text-foreground text-sm transition-colors duration-200'
    >
      {label}
    </Link>
  )
}

// Renders User Agreement / Privacy Policy links inline with the parent's
// copyright row when either is configured in System Settings → Site. Emits
// fragmented siblings so the parent flex container's gap controls spacing.
function LegalLinks() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const items: { key: string; label: string; href: string }[] = []
  if (status?.user_agreement_enabled) {
    items.push({
      key: 'user-agreement',
      label: t('User Agreement'),
      href: '/user-agreement',
    })
  }
  if (status?.privacy_policy_enabled) {
    items.push({
      key: 'privacy-policy',
      label: t('Privacy Policy'),
      href: '/privacy-policy',
    })
  }
  if (items.length === 0) {
    return null
  }
  // On narrow screens the links move under the copyright line, so the dot
  // that joins them only shows from sm upwards.
  return (
    <>
      <span
        aria-hidden='true'
        className='text-muted-foreground/30 hidden sm:inline'
      >
        ·
      </span>
      <span className='flex items-center gap-x-2.5'>
        {items.map((item, index) => (
          <Fragment key={item.key}>
            {index > 0 && (
              <span aria-hidden='true' className='text-muted-foreground/30'>
                ·
              </span>
            )}
            <Link
              to={item.href}
              className='hover:text-foreground transition-colors duration-200'
            >
              {item.label}
            </Link>
          </Fragment>
        ))}
      </span>
    </>
  )
}

// Copyright, legal links, the licence notice and the version stamp, centered.
// The attribution is a legal notice, not marketing copy: the upstream NOTICE
// (AGPL-3.0 section 7(b)) requires its line and the link to the original
// project to stay visible in the footer, so it is shown small but readable.
function FooterLegalBlock(props: { name: string; copyright: string }) {
  const currentYear = new Date().getFullYear()
  return (
    <div className='flex flex-col items-center gap-3 text-center'>
      <div className='text-muted-foreground flex flex-col items-center justify-center gap-x-2.5 gap-y-1.5 text-xs sm:flex-row'>
        <span>
          &copy; {currentYear} {props.name}. {props.copyright}
        </span>
        <LegalLinks />
      </div>
      <AttributionNotice className='text-muted-foreground text-[11px] leading-relaxed' />
      <BuildStamp />
    </div>
  )
}

function BuildStamp() {
  if (!BUILD_STAMP) return null
  return (
    <span
      className='text-muted-foreground font-mono text-[10px] tracking-wide tabular-nums'
      data-testid='build-stamp'
    >
      {BUILD_STAMP}
    </span>
  )
}

export function Footer(props: FooterProps) {
  const { t } = useTranslation()
  const {
    systemName,
    logo: systemLogo,
    footerHtml,
    demoSiteEnabled,
  } = useSystemConfig()

  const displayLogo = systemLogo || props.logo || '/logo.png'
  const displayName = systemName || props.name || DEFAULT_SYSTEM_NAME
  const isDemoSiteMode = Boolean(demoSiteEnabled)
  const copyright = props.copyright ?? t('footer.defaultCopyright')

  // Only our own documentation page is offered; the upstream project's wiki and
  // its related projects are not written for our customers, so those links are
  // gone.
  const fallbackColumns = useMemo<FooterColumnProps[]>(
    () => [
      {
        title: t('footer.columns.docs.title'),
        links: [{ text: t('Docs'), href: '/docs' }],
      },
    ],
    [t]
  )

  const displayColumns = props.columns ?? fallbackColumns

  if (props.variant === 'compact') {
    return (
      <footer
        className={cn(
          'border-border/40 shrink-0 border-t px-4 py-3',
          props.className
        )}
      >
        <TrustBlock centered className='mb-3' />
        <FooterLegalBlock name={displayName} copyright={copyright} />
      </footer>
    )
  }

  return (
    <footer
      className={cn('border-border/40 relative z-10 border-t', props.className)}
    >
      <div className='mx-auto flex max-w-3xl flex-col items-center gap-8 px-6 py-12 text-center md:py-14'>
        {footerHtml ? (
          <div
            className='custom-footer text-muted-foreground text-sm'
            dangerouslySetInnerHTML={{ __html: footerHtml }}
          />
        ) : (
          <div className='flex flex-col items-center gap-3'>
            <Link to='/' className='flex items-center gap-2.5'>
              <img
                src={displayLogo}
                alt=''
                className='size-8 rounded-lg object-contain'
              />
              <span className='text-base font-semibold tracking-tight'>
                {displayName}
              </span>
            </Link>
            <p className='text-muted-foreground text-xs'>
              {t('Powerful API Management Platform')}
            </p>
          </div>
        )}

        {isDemoSiteMode && (
          <div className='flex flex-wrap justify-center gap-x-12 gap-y-6'>
            {displayColumns.map((column) => (
              <div key={column.title}>
                <p className='text-muted-foreground mb-3 text-xs font-medium tracking-wider uppercase'>
                  {t(column.title)}
                </p>
                <ul className='space-y-2.5'>
                  {column.links.map((link) => (
                    <li key={link.href}>
                      <FooterLinkItem link={link} />
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        )}

        <div
          aria-hidden='true'
          className='via-border h-px w-full max-w-md bg-gradient-to-r from-transparent to-transparent'
        />

        <TrustBlock centered />

        <FooterLegalBlock name={displayName} copyright={copyright} />
      </div>
    </footer>
  )
}
