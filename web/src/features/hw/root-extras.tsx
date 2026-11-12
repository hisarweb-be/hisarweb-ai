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
import { lazy, Suspense } from 'react'

// Not needed for the first paint, so they stay out of the first JS bundle
// (HIS-7996): the assistant widget loads after the page, the devtools only in
// a development build.
const AssistantWidget = lazy(() =>
  import('./admin-agent/assistant-widget').then((m) => ({
    default: m.AssistantWidget,
  }))
)

const Devtools = lazy(() =>
  Promise.all([
    import('@tanstack/react-query-devtools'),
    import('@tanstack/react-router-devtools'),
  ]).then(([query, router]) => ({
    default: () => (
      <>
        <query.ReactQueryDevtools buttonPosition='bottom-left' />
        <router.TanStackRouterDevtools position='bottom-right' />
      </>
    ),
  }))
)

export function RootExtras() {
  return (
    <Suspense fallback={null}>
      <AssistantWidget />
      {import.meta.env.MODE === 'development' && <Devtools />}
    </Suspense>
  )
}
