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
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { CopyButton } from '@/components/copy-button'
import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import { useDocsGuideText } from '../docs-guide-texts'
import { MODEL_HAIKU, OPENAI_BASE_URL } from '../docs-tool-guides'
import { useHwText, type HwTextKey } from '../texts'
import { AiSetupGuide, KEY_STEP_ANCHOR } from './docs-ai-setup'
import { ToolGuides } from './docs-tool-guides'

/** Static video of the same steps and its still frame. */
const GUIDE_VIDEO_SRC = '/rehber/hisarweb-ai-rehber.mp4'
const GUIDE_VIDEO_POSTER = '/rehber/hisarweb-ai-rehber.jpg'

/** The cheapest model, used in every example so the samples work on a trial account. */
const EXAMPLE_MODEL = MODEL_HAIKU

const CHAT_COMPLETIONS_EXAMPLE = `curl ${OPENAI_BASE_URL}/chat/completions \\
  -H "Authorization: Bearer $HISARWEB_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${EXAMPLE_MODEL}",
    "messages": [
      { "role": "user", "content": "Merhaba!" }
    ]
  }'`

const MESSAGES_EXAMPLE = `curl ${OPENAI_BASE_URL}/messages \\
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

const STREAM_EXAMPLE = `curl ${OPENAI_BASE_URL}/chat/completions \\
  -H "Authorization: Bearer $HISARWEB_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${EXAMPLE_MODEL}",
    "stream": true,
    "messages": [
      { "role": "user", "content": "Bana kısa bir şiir yaz." }
    ]
  }'`

function GuideImage(props: { src: string; alt: string }) {
  return (
    <img
      src={props.src}
      alt={props.alt}
      loading='lazy'
      className='h-auto w-full rounded-lg border'
    />
  )
}

function GuideStep(props: {
  number: number
  title: string
  id?: string
  children: React.ReactNode
}) {
  const guide = useDocsGuideText()
  return (
    <li id={props.id} className='scroll-mt-24 space-y-3'>
      <div className='space-y-1'>
        <p className='text-primary text-xs font-medium tracking-wide uppercase'>
          {guide('step', { number: props.number })}
        </p>
        <h3 className='text-lg font-semibold'>{props.title}</h3>
      </div>
      <div className='space-y-3 text-sm'>{props.children}</div>
    </li>
  )
}

function Paragraph(props: { children: React.ReactNode }) {
  return <p className='text-muted-foreground'>{props.children}</p>
}

/** The plain-language walk from sign-up to a first answer in Claude Code. */
function BeginnerGuide() {
  const { t } = useTranslation()
  const guide = useDocsGuideText()

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className='text-base'>{guide('whatTitle')}</CardTitle>
        </CardHeader>
        <CardContent className='space-y-3 text-sm'>
          <Paragraph>{guide('whatBody')}</Paragraph>
          <p className='font-medium'>{guide('whatFree')}</p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className='text-base'>{guide('videoTitle')}</CardTitle>
        </CardHeader>
        <CardContent className='space-y-3'>
          <p className='text-muted-foreground text-sm'>{guide('videoBody')}</p>
          <video
            controls
            preload='none'
            poster={GUIDE_VIDEO_POSTER}
            className='w-full rounded-lg border'
          >
            <source src={GUIDE_VIDEO_SRC} type='video/mp4' />
            {guide('videoFallback')}
          </video>
        </CardContent>
      </Card>

      <section className='space-y-6'>
        <h2 className='text-2xl font-semibold tracking-tight'>
          {guide('stepsTitle')}
        </h2>
        <ol className='space-y-10'>
          <GuideStep number={1} title={guide('step1Title')}>
            <Paragraph>{guide('step1Body')}</Paragraph>
            <GuideImage
              src='/rehber/adim-1-kayit.png'
              alt={guide('step1Alt')}
            />
          </GuideStep>
          <GuideStep number={2} title={guide('step2Title')}>
            <Paragraph>{guide('step2Body')}</Paragraph>
            <GuideImage
              src='/rehber/adim-2-startbonus.png'
              alt={guide('step2Alt')}
            />
          </GuideStep>
          <GuideStep
            number={3}
            id={KEY_STEP_ANCHOR}
            title={guide('step3Title')}
          >
            <Paragraph>{guide('step3Body')}</Paragraph>
            <GuideImage
              src='/rehber/adim-2-anahtar-olustur.png'
              alt={guide('step3Alt')}
            />
            <Paragraph>{guide('step3Body2')}</Paragraph>
            <GuideImage
              src='/rehber/adim-3-anahtar-listesi.png'
              alt={guide('step3Alt2')}
            />
          </GuideStep>
          <GuideStep number={4} title={guide('step4Title')}>
            <Paragraph>{guide('step4Body')}</Paragraph>
            {/* Plain anchor: the guide must render without the router tree. */}
            <a href='/satin-al' className='text-primary hover:underline'>
              {guide('step4Link')}
            </a>
          </GuideStep>
          <GuideStep number={5} title={guide('step5Title')}>
            <Paragraph>{guide('step5Body')}</Paragraph>
            <GuideImage
              src='/rehber/adim-4-playground.png'
              alt={guide('step5Alt')}
            />
          </GuideStep>
          <GuideStep number={6} title={guide('step6Title')}>
            <ToolGuides />
          </GuideStep>
        </ol>
      </section>

      <AiSetupGuide />

      <Card>
        <CardHeader>
          <CardTitle className='text-base'>{t('Costs and refunds')}</CardTitle>
        </CardHeader>
        <CardContent className='space-y-3 text-sm'>
          <Paragraph>
            {t(
              'HisarWeb pays the payment and currency conversion costs. You get the full credit for what you pay, with no surcharge or deduction.'
            )}
          </Paragraph>
          <Paragraph>
            {t(
              'On a refund, the payment processing and currency conversion costs that HisarWeb paid are deducted from the refunded amount.'
            )}
          </Paragraph>
        </CardContent>
      </Card>
    </>
  )
}

function DocSection(props: {
  titleKey: HwTextKey
  text: (key: HwTextKey) => string
  children?: React.ReactNode
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className='text-base'>
          {props.text(props.titleKey)}
        </CardTitle>
      </CardHeader>
      <CardContent className='space-y-3'>{props.children}</CardContent>
    </Card>
  )
}

/** `/docs`: the beginner guide first, the API reference folded away below it. */
export function DocsPage() {
  const text = useHwText()
  const guide = useDocsGuideText()

  // A link such as /docs#kurulumu-yapay-zekaya-yaptirin arrives before the
  // page has rendered, so the browser cannot jump there on its own.
  useEffect(() => {
    const id = decodeURIComponent(window.location.hash.slice(1))
    if (!id) return
    // The id comes from the address bar, so it must not be read as a selector.
    // eslint-disable-next-line unicorn/prefer-query-selector
    const target = document.getElementById(id)
    if (typeof target?.scrollIntoView === 'function') {
      target.scrollIntoView({ block: 'start' })
    }
  }, [])

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition className='container mx-auto max-w-3xl space-y-6 px-4 pt-24 pb-12'>
        <header className='space-y-2'>
          <h1 className='text-3xl font-semibold tracking-tight'>
            {text('docsTitle')}
          </h1>
          <p className='text-muted-foreground'>{text('docsLead')}</p>
        </header>

        <BeginnerGuide />

        <Accordion className='w-full'>
          <AccordionItem value='developers' className='border-border/60'>
            <AccordionTrigger className='text-start hover:no-underline'>
              <span className='text-lg font-semibold'>{guide('devTitle')}</span>
            </AccordionTrigger>
            <AccordionContent className='space-y-6'>
              <p className='text-muted-foreground text-sm'>
                {guide('devLead')}
              </p>

              <DocSection text={text} titleKey='docsBaseUrlTitle'>
                <p className='text-muted-foreground text-sm'>
                  {text('docsBaseUrlBody')}
                </p>
                <div className='bg-muted/50 flex items-center justify-between gap-3 rounded-lg border px-3 py-2'>
                  <code className='min-w-0 truncate font-mono text-sm'>
                    {OPENAI_BASE_URL}
                  </code>
                  <CopyButton value={OPENAI_BASE_URL} />
                </div>
              </DocSection>

              <DocSection text={text} titleKey='docsKeyTitle'>
                <p className='text-muted-foreground text-sm'>
                  {text('docsKeyBody')}
                </p>
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
            </AccordionContent>
          </AccordionItem>
        </Accordion>

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
            <li>
              <a href='/vibe-kodlama' className='text-primary hover:underline'>
                {text('docsLinkVibe')}
              </a>
            </li>
          </ul>
        </DocSection>
      </PageTransition>
    </PublicLayout>
  )
}
