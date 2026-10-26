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
import '@/styles/hw-landing.css'
import '@/styles/hw-cinematic.css'
import type { TopNavLink } from '../types'
import { Footer } from './footer'
import { PublicHeader, type PublicHeaderProps } from './public-header'

type PublicLayoutProps = {
  children: React.ReactNode
  showMainContainer?: boolean
  /** Aurora backdrop behind the page top; the landing page has its own hero. */
  backdrop?: boolean
  navContent?: React.ReactNode
  headerProps?: Omit<PublicHeaderProps, 'navContent'>
  navLinks?: TopNavLink[]
  showThemeSwitch?: boolean
  showAuthButtons?: boolean
  showNotifications?: boolean
  logo?: React.ReactNode
  siteName?: string
}

export function PublicLayout(props: PublicLayoutProps) {
  return (
    <div
      className='hwl hwp bg-background text-foreground relative min-h-svh overflow-x-clip'
      data-testid='public-layout'
    >
      {props.backdrop !== false && (
        <div
          aria-hidden='true'
          className='hwp-backdrop'
          data-testid='public-backdrop'
        >
          <div className='hwl-aurora'>
            <span className='hwl-blob hwl-blob-1' />
            <span className='hwl-blob hwl-blob-2' />
            <span className='hwl-blob hwl-blob-3' />
          </div>
          <div className='hwl-grid' />
          <div className='hwl-grain' />
        </div>
      )}
      <PublicHeader
        navContent={props.navContent}
        navLinks={props.navLinks}
        showThemeSwitch={props.showThemeSwitch}
        showAuthButtons={props.showAuthButtons}
        showNotifications={props.showNotifications}
        logo={props.logo}
        siteName={props.siteName}
        {...props.headerProps}
      />

      {props.showMainContainer !== false ? (
        <main className='container px-4 py-6 pt-24 md:px-4'>
          {props.children}
        </main>
      ) : (
        <main>{props.children}</main>
      )}
      {/* Every public page ends with the same footer. */}
      <Footer />
    </div>
  )
}
