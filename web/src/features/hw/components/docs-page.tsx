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
import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { CopyButton } from '@/components/copy-button'
import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { PageTransition } from '@/components/page-transition'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import { useHwText, type HwTextKey } from '../texts'

/** The public API address every example on this page starts from. */
export const HW_API_BASE_URL = 'https://api.hisarweb.be/v1'

/** The cheapest model, used in every example so the samples work on a trial account. */
const EXAMPLE_MODEL = 'claude-haiku-4-5'

const CHAT_COMPLETIONS_EXAMPLE = `curl ${HW_API_BASE_URL}/chat/completions \\
  -H "Authorization: Bearer $HISARWEB_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${EXAMPLE_MODEL}",
    "messages": [
      { "role": "user", "content": "Merhaba!" }
    ]
  }'`

const MESSAGES_EXAMPLE = `curl ${HW_API_BASE_URL}/messages \\
  -H "x-api-key: $HISARWEB_API_KEY" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${EXAMPLE_MODEL}",
    "max_tokens": 256,
    "messages": [
      { "role": "user", "content": "Merhaba!" }
    ]
  }'`

const STREAM_EXAMPLE = `curl ${HW_API_BASE_URL}/chat/completions \\
  -H "Authorization: Bearer $HISARWEB_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${EXAMPLE_MODEL}",
    "stream": true,
    "messages": [
      { "role": "user", "content": "Bana kısa bir şiir yaz." }
    ]
  }'`

function DocSection(props: {
  titleKey: HwTextKey
  text: (key: HwTextKey) => string
  children?: React.ReactNode
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className='text-base'>{props.text(props.titleKey)}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-3'>{props.children}</CardContent>
    </Card>
  )
}

/** `/docs`: the public API guide, readable without a login. */
export function DocsPage() {
  const text = useHwText()

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition className='container mx-auto max-w-3xl space-y-6 px-4 pt-24 pb-12'>
        <header className='space-y-2'>
          <h1 className='text-3xl font-semibold tracking-tight'>
            {text('docsTitle')}
          </h1>
          <p className='text-muted-foreground'>{text('docsLead')}</p>
        </header>

        <DocSection text={text} titleKey='docsBaseUrlTitle'>
          <p className='text-muted-foreground text-sm'>
            {text('docsBaseUrlBody')}
          </p>
          <div className='bg-muted/50 flex items-center justify-between gap-3 rounded-lg border px-3 py-2'>
            <code className='min-w-0 truncate font-mono text-sm'>
              {HW_API_BASE_URL}
            </code>
            <CopyButton value={HW_API_BASE_URL} />
          </div>
        </DocSection>

        <DocSection text={text} titleKey='docsKeyTitle'>
          <p className='text-muted-foreground text-sm'>{text('docsKeyBody')}</p>
        </DocSection>

        <DocSection text={text} titleKey='docsChatTitle'>
          <CodeBlock code={CHAT_COMPLETIONS_EXAMPLE} language='bash'>
            <CodeBlockCopyButton />
          </CodeBlock>
        </DocSection>

        <DocSection text={text} titleKey='docsMessagesTitle'>
          <CodeBlock code={MESSAGES_EXAMPLE} language='bash'>
            <CodeBlockCopyButton />
          </CodeBlock>
        </DocSection>

        <DocSection text={text} titleKey='docsStreamTitle'>
          <p className='text-muted-foreground text-sm'>
            {text('docsStreamBody')}
          </p>
          <CodeBlock code={STREAM_EXAMPLE} language='bash'>
            <CodeBlockCopyButton />
          </CodeBlock>
        </DocSection>

        <DocSection text={text} titleKey='docsModelsTitle'>
          <p className='text-muted-foreground text-sm'>
            {text('docsModelsBody')}
          </p>
          <ul className='text-muted-foreground list-disc space-y-1.5 pl-5 text-sm'>
            <li>{text('docsModelsFree')}</li>
            <li>{text('docsModelsPaid')}</li>
          </ul>
        </DocSection>

        <DocSection text={text} titleKey='docsErrorsTitle'>
          <ul className='text-muted-foreground list-disc space-y-1.5 pl-5 text-sm'>
            <li>{text('docsError401')}</li>
            <li>{text('docsError402')}</li>
            <li>{text('docsError429')}</li>
          </ul>
        </DocSection>

        <DocSection text={text} titleKey='docsLinksTitle'>
          {/* Plain anchors: this guide is a public, self-contained page and
              must render without the authenticated router tree. */}
          <ul className='text-sm'>
            <li>
              <a href='/durum' className='text-primary hover:underline'>
                {text('docsLinkStatus')}
              </a>
            </li>
            <li>
              <a
                href='/fiyat-karsilastirma'
                className='text-primary hover:underline'
              >
                {text('docsLinkCompare')}
              </a>
            </li>
          </ul>
        </DocSection>
      </PageTransition>
      <Footer />
    </PublicLayout>
  )
}
