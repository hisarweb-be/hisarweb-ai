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
import type { ReactNode } from 'react'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { PageTransition } from '@/components/page-transition'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import {
  ANTHROPIC_BASE_URL,
  KEY_PLACEHOLDER,
  MODEL_DEFAULT,
  MODEL_HAIKU,
  MODEL_OPUS,
  OPENAI_BASE_URL,
} from '../docs-tool-guides'
import { useVibeText, type VibeTextKey } from '../vibe-texts'

type Translate = ReturnType<typeof useVibeText>

/**
 * The film for beginners is made in a separate task. Until its file exists
 * this stays `undefined` and the page shows no player at all. Fill in the
 * paths here once `public/film/` holds the video and its poster.
 */
export const VIBE_FILM: { src: string; poster: string } | undefined = undefined

const CLAUDE_BASH = `export ANTHROPIC_BASE_URL="${ANTHROPIC_BASE_URL}"
export ANTHROPIC_AUTH_TOKEN="${KEY_PLACEHOLDER}"
export ANTHROPIC_MODEL="${MODEL_DEFAULT}"
export ANTHROPIC_DEFAULT_HAIKU_MODEL="${MODEL_HAIKU}"
export CLAUDE_CODE_SUBAGENT_MODEL="${MODEL_HAIKU}"
claude`

const CLAUDE_POWERSHELL = `$env:ANTHROPIC_BASE_URL = "${ANTHROPIC_BASE_URL}"
$env:ANTHROPIC_AUTH_TOKEN = "${KEY_PLACEHOLDER}"
$env:ANTHROPIC_MODEL = "${MODEL_DEFAULT}"
$env:ANTHROPIC_DEFAULT_HAIKU_MODEL = "${MODEL_HAIKU}"
$env:CLAUDE_CODE_SUBAGENT_MODEL = "${MODEL_HAIKU}"
claude`

const AIDER_COMMAND = `aider --architect --model openai/${MODEL_OPUS} --editor-model openai/${MODEL_DEFAULT}`

/** Same key, two models: only the `model` field changes between the requests. */
const TWO_MODELS_EXAMPLE = ['claude-opus-5-5', 'claude-sonnet-5-5']
  .map(
    (model) => `curl ${OPENAI_BASE_URL}/chat/completions \\
  -H "Authorization: Bearer ${KEY_PLACEHOLDER}" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "${model}", "messages": [{"role": "user", "content": "Merhaba"}]}'`
  )
  .join('\n\n')

const EXPERT_KEYS: VibeTextKey[] = [
  'expertItem1',
  'expertItem2',
  'expertItem3',
  'expertItem4',
]
const WORK_KEYS: VibeTextKey[] = [
  'workItem1',
  'workItem2',
  'workItem3',
  'workItem4',
]
const CACHE_KEYS: VibeTextKey[] = [
  'cacheTip1',
  'cacheTip2',
  'cacheTip3',
  'cacheTip4',
  'cacheTip5',
]
const PROJECT_KEYS: VibeTextKey[] = [
  'fitYesProject1',
  'fitYesProject2',
  'fitYesProject3',
  'fitYesProject4',
  'fitYesProject5',
  'fitYesProject6',
]

function Section(props: { title: string; children?: ReactNode }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className='text-xl'>{props.title}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-3'>{props.children}</CardContent>
    </Card>
  )
}

function Paragraph(props: { children: ReactNode }) {
  return (
    <p className='text-muted-foreground text-sm leading-relaxed'>
      {props.children}
    </p>
  )
}

function BulletList(props: { text: Translate; keys: VibeTextKey[] }) {
  return (
    <ul className='text-muted-foreground list-disc space-y-1.5 ps-5 text-sm'>
      {props.keys.map((key) => (
        <li key={key}>{props.text(key)}</li>
      ))}
    </ul>
  )
}

function Layers(props: { text: Translate }) {
  const text = props.text
  return (
    <div className='grid gap-3 sm:grid-cols-2'>
      <div className='space-y-2 rounded-lg border p-4'>
        <h3 className='text-base font-semibold'>{text('expertTitle')}</h3>
        <Paragraph>{text('expertBody')}</Paragraph>
        <BulletList text={text} keys={EXPERT_KEYS} />
      </div>
      <div className='space-y-2 rounded-lg border p-4'>
        <h3 className='text-base font-semibold'>{text('workTitle')}</h3>
        <Paragraph>{text('workBody')}</Paragraph>
        <BulletList text={text} keys={WORK_KEYS} />
      </div>
    </div>
  )
}

function ToolRow(props: { name: string; children?: ReactNode; body: string }) {
  return (
    <div className='space-y-2 rounded-lg border p-4'>
      <h3 className='text-base font-semibold'>{props.name}</h3>
      <Paragraph>{props.body}</Paragraph>
      {props.children}
    </div>
  )
}

function Tools(props: { text: Translate }) {
  const text = props.text
  return (
    <Section title={text('toolsTitle')}>
      <Paragraph>{text('toolsBody')}</Paragraph>
      <div className='space-y-3'>
        <ToolRow name={text('toolClaudeName')} body={text('toolClaudeBody')}>
          <p className='text-sm font-medium'>{text('toolClaudeShellBash')}</p>
          <CodeBlock code={CLAUDE_BASH} language='bash'>
            <CodeBlockCopyButton />
          </CodeBlock>
          <p className='text-sm font-medium'>
            {text('toolClaudeShellPowershell')}
          </p>
          <CodeBlock code={CLAUDE_POWERSHELL} language='powershell'>
            <CodeBlockCopyButton />
          </CodeBlock>
        </ToolRow>
        <ToolRow name={text('toolClineName')} body={text('toolClineBody')} />
        <ToolRow name={text('toolKiloName')} body={text('toolKiloBody')} />
        <ToolRow
          name={text('toolOpencodeName')}
          body={text('toolOpencodeBody')}
        />
        <ToolRow name={text('toolAiderName')} body={text('toolAiderBody')}>
          <CodeBlock code={AIDER_COMMAND} language='bash'>
            <CodeBlockCopyButton />
          </CodeBlock>
        </ToolRow>
        <ToolRow name={text('toolCursorName')} body={text('toolCursorBody')} />
      </div>
      <Paragraph>{text('toolRouteNote')}</Paragraph>
      <p className='text-sm'>
        <a href='/docs' className='text-primary hover:underline'>
          {text('toolsDocsLink')}
        </a>
      </p>
    </Section>
  )
}

function Fit(props: { text: Translate }) {
  const text = props.text
  return (
    <Section title={text('fitTitle')}>
      <div className='grid gap-3 sm:grid-cols-2'>
        <div className='space-y-2 rounded-lg border p-4'>
          <h3 className='text-base font-semibold'>{text('fitYesTitle')}</h3>
          <p className='text-sm font-medium'>{text('fitYesProjectsTitle')}</p>
          <BulletList text={text} keys={PROJECT_KEYS} />
          <p className='text-sm font-medium'>{text('fitYesPlatformsTitle')}</p>
          <Paragraph>{text('fitYesPlatforms')}</Paragraph>
        </div>
        <div className='space-y-2 rounded-lg border p-4'>
          <h3 className='text-base font-semibold'>{text('fitNoTitle')}</h3>
          <BulletList text={text} keys={['fitNo1', 'fitNo2']} />
        </div>
      </div>
    </Section>
  )
}

/** Renders nothing while there is no film, so the page has no empty box. */
export function VibeFilm(props: {
  film?: { src: string; poster: string }
  title: string
  label: string
}) {
  if (!props.film) return null
  return (
    <Section title={props.title}>
      <div className='overflow-hidden rounded-xl border bg-black'>
        <video
          controls
          playsInline
          preload='metadata'
          poster={props.film.poster}
          src={props.film.src}
          aria-label={props.label}
          className='block aspect-video w-full'
        />
      </div>
    </Section>
  )
}

/** `/vibe-kodlama`: how a beginner splits the work between subscription and key. */
export function VibePage() {
  const text = useVibeText()

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition className='container mx-auto max-w-3xl space-y-6 px-4 pt-24 pb-12'>
        <header className='space-y-2'>
          <h1 className='text-3xl font-semibold tracking-tight'>
            {text('title')}
          </h1>
          <p className='text-muted-foreground'>{text('lead')}</p>
        </header>

        <Section title={text('forWhoTitle')}>
          <Paragraph>{text('forWhoBody')}</Paragraph>
        </Section>

        <Section title={text('ruleTitle')}>
          <p className='text-lg font-semibold'>{text('ruleSentence')}</p>
          <Paragraph>{text('ruleBody')}</Paragraph>
          <Layers text={text} />
          <h3 className='pt-2 text-base font-semibold'>{text('priceTitle')}</h3>
          <Paragraph>{text('priceBody')}</Paragraph>
          <ul className='space-y-1 text-sm'>
            <li>
              <a
                href='https://claude.com/pricing'
                target='_blank'
                rel='noreferrer noopener'
                className='text-primary hover:underline'
              >
                {text('priceLinkClaude')}
              </a>
            </li>
            <li>
              <a
                href='https://chatgpt.com/pricing'
                target='_blank'
                rel='noreferrer noopener'
                className='text-primary hover:underline'
              >
                {text('priceLinkChatgpt')}
              </a>
            </li>
          </ul>
          <Paragraph>{text('priceMeasured')}</Paragraph>
        </Section>

        <Section title={text('oneKeyTitle')}>
          <Paragraph>{text('oneKeyBody')}</Paragraph>
          <Paragraph>{text('oneKeyCaption')}</Paragraph>
          <CodeBlock code={TWO_MODELS_EXAMPLE} language='bash'>
            <CodeBlockCopyButton />
          </CodeBlock>
          <Alert className='border-amber-500/50 bg-amber-500/10'>
            <AlertTitle>{text('warnTitle')}</AlertTitle>
            <AlertDescription>{text('warnBody')}</AlertDescription>
          </Alert>
        </Section>

        <Section title={text('cacheTitle')}>
          <Paragraph>{text('cacheBody')}</Paragraph>
          <BulletList text={text} keys={CACHE_KEYS} />
          <Paragraph>{text('cacheNote')}</Paragraph>
        </Section>

        <Tools text={text} />

        <Section title={text('promptTitle')}>
          <Paragraph>{text('promptBody')}</Paragraph>
          <CodeBlock code={text('promptText')} language='markdown'>
            <CodeBlockCopyButton />
          </CodeBlock>
        </Section>

        <Fit text={text} />

        <VibeFilm
          film={VIBE_FILM}
          title={text('filmTitle')}
          label={text('filmLabel')}
        />

        <Section title={text('ctaTitle')}>
          <Paragraph>{text('ctaBody')}</Paragraph>
          <div className='flex flex-wrap gap-3'>
            <Button render={<a href='/sign-up' />}>{text('ctaButton')}</Button>
            <Button variant='outline' render={<a href='/docs' />}>
              {text('ctaDocs')}
            </Button>
          </div>
        </Section>
      </PageTransition>
      <Footer />
    </PublicLayout>
  )
}
