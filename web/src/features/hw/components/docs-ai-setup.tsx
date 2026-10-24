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
  ArrowRight,
  ClipboardPaste,
  KeyRound,
  SlidersHorizontal,
  SquareTerminal,
  type LucideIcon,
} from 'lucide-react'
import type { ReactNode } from 'react'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import { AI_SETUP_PROMPTS } from '../docs-ai-setup-prompts'
import { useAiSetupText, type AiSetupTextKey } from '../docs-ai-setup-texts'
import {
  KEY_PLACEHOLDER,
  MODEL_CODEX,
  MODEL_DEFAULT,
  MODEL_HAIKU,
  MODEL_OPUS,
} from '../docs-tool-guides'
import { useHwLanguage } from '../texts'

/** Anchor of this part; the vibe coding page links straight to it. */
export const AI_SETUP_ANCHOR = 'kurulumu-yapay-zekaya-yaptirin'

/** Anchor of step 3 (create an API key) on the same page. */
export const KEY_STEP_ANCHOR = 'adim-3'

const FLOW: readonly { key: AiSetupTextKey; icon: LucideIcon }[] = [
  { key: 'flow1', icon: KeyRound },
  { key: 'flow2', icon: SquareTerminal },
  { key: 'flow3', icon: SlidersHorizontal },
  { key: 'flow4', icon: ClipboardPaste },
]

const TOOL_KEYS: AiSetupTextKey[] = ['toolClaude', 'toolCodex', 'toolVscode']
const MODE_KEYS: AiSetupTextKey[] = [
  'modeClaudeDesktop',
  'modeVscode',
  'modeCli',
  'modeCodex',
]

type Translate = ReturnType<typeof useAiSetupText>

function Paragraph(props: { children: ReactNode }) {
  return <p className='text-muted-foreground'>{props.children}</p>
}

function List(props: { items: string[] }) {
  return (
    <ul className='text-muted-foreground list-disc space-y-1.5 ps-5'>
      {props.items.map((item) => (
        <li key={item}>{item}</li>
      ))}
    </ul>
  )
}

function SubStep(props: { title: string; children: ReactNode }) {
  return (
    <div className='space-y-3'>
      <h3 className='text-base font-semibold'>{props.title}</h3>
      {props.children}
    </div>
  )
}

/** The four steps as a small flow: one row on wide screens, a column on phones. */
function Flow(props: { text: Translate }) {
  return (
    <figure
      aria-label={props.text('flowLabel')}
      className='bg-muted/40 rounded-lg border p-4'
    >
      <ol className='flex flex-col gap-2 sm:flex-row sm:items-stretch'>
        {FLOW.map((step, index) => (
          <li
            key={step.key}
            className='flex flex-1 flex-col items-center gap-2 sm:flex-row'
          >
            <div className='bg-background flex w-full flex-1 items-center gap-3 rounded-md border p-3 sm:flex-col sm:text-center'>
              <span className='bg-primary/10 text-primary flex size-9 shrink-0 items-center justify-center rounded-full'>
                <step.icon className='size-4' aria-hidden='true' />
              </span>
              <span className='text-sm font-medium'>
                {index + 1}. {props.text(step.key)}
              </span>
            </div>
            {index < FLOW.length - 1 && (
              <ArrowRight
                className='text-muted-foreground size-4 shrink-0 rotate-90 sm:rotate-0'
                aria-hidden='true'
              />
            )}
          </li>
        ))}
      </ol>
    </figure>
  )
}

/**
 * The beginner route on `/docs`: create a key, open the tool, pick model and
 * permission mode, then paste a prompt that makes the tool set itself up.
 */
export function AiSetupGuide() {
  const text = useAiSetupText()
  const prompts = AI_SETUP_PROMPTS[useHwLanguage()]

  return (
    <Card id={AI_SETUP_ANCHOR} className='scroll-mt-24'>
      <CardHeader>
        <CardTitle className='text-base'>{text('title')}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-6 text-sm'>
        <div className='space-y-3'>
          <Paragraph>{text('lead')}</Paragraph>
          <p className='font-medium'>{text('needsLogin')}</p>
        </div>

        <Flow text={text} />

        <SubStep title={text('keyTitle')}>
          <Paragraph>{text('keyBody')}</Paragraph>
          <p>
            <a
              href={`#${KEY_STEP_ANCHOR}`}
              className='text-primary hover:underline'
            >
              {text('keyLink')}
            </a>
          </p>
        </SubStep>

        <SubStep title={text('toolTitle')}>
          <List items={TOOL_KEYS.map((key) => text(key))} />
          <Paragraph>{text('toolFolder')}</Paragraph>
        </SubStep>

        <SubStep title={text('modelTitle')}>
          <Paragraph>{text('modelBody')}</Paragraph>
          <List
            items={[
              text('modelDefault', { model: MODEL_DEFAULT }),
              text('modelHaiku', { model: MODEL_HAIKU }),
              text('modelOpus', { model: MODEL_OPUS }),
              text('modelCodex', { model: MODEL_CODEX }),
            ]}
          />
          <Paragraph>{text('modeBody')}</Paragraph>
          <List items={MODE_KEYS.map((key) => text(key))} />
          <Alert className='border-amber-500/50 bg-amber-500/10'>
            <AlertTitle>{text('riskTitle')}</AlertTitle>
            <AlertDescription>{text('riskBody')}</AlertDescription>
          </Alert>
        </SubStep>

        <SubStep title={text('promptTitle')}>
          <Paragraph>
            {text('promptBody', { placeholder: KEY_PLACEHOLDER })}
          </Paragraph>
          <p className='font-medium'>{text('promptClaudeTitle')}</p>
          <CodeBlock code={prompts.claude} language='markdown'>
            <CodeBlockCopyButton />
          </CodeBlock>
          <p className='font-medium'>{text('promptCodexTitle')}</p>
          <CodeBlock code={prompts.codex} language='markdown'>
            <CodeBlockCopyButton />
          </CodeBlock>
          <Paragraph>{text('promptAfter')}</Paragraph>
        </SubStep>
      </CardContent>
    </Card>
  )
}
