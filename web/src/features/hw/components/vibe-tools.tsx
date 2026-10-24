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
import { Sparkles } from 'lucide-react'
import type { ReactNode } from 'react'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'

import {
  ANTHROPIC_BASE_URL,
  KEY_PLACEHOLDER,
  MODEL_DEFAULT,
  MODEL_HAIKU,
  MODEL_OPUS,
  type GuideToolId,
} from '../docs-tool-guides'
import type { VibeTextKey } from '../vibe-texts'
import { AI_SETUP_ANCHOR } from './docs-ai-setup'
import { TOOL_GUIDE_HASH_PREFIX } from './docs-tool-guides'
import {
  BulletList,
  NewTabLink,
  Paragraph,
  Section,
  type VibeTranslate,
} from './vibe-parts'

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

const USE_KEYS: VibeTextKey[] = [
  'fitUse1',
  'fitUse2',
  'fitUse3',
  'fitUse4',
  'fitUse5',
  'fitUse6',
  'fitUse7',
  'fitUse8',
  'fitUse9',
  'fitUse10',
  'fitUse11',
  'fitUse12',
  'fitUse13',
]

/** The `/docs` address that opens the setup guide of one tool. */
function toolGuideHref(tool: GuideToolId): string {
  return `/docs#${TOOL_GUIDE_HASH_PREFIX}${tool}`
}

function ToolRow(props: {
  tool: GuideToolId
  name: string
  body: string
  children?: ReactNode
}) {
  return (
    <div className='space-y-2 rounded-lg border p-4'>
      <h3 className='text-base font-semibold'>
        <NewTabLink href={toolGuideHref(props.tool)}>{props.name}</NewTabLink>
      </h3>
      <Paragraph>{props.body}</Paragraph>
      {props.children}
    </div>
  )
}

export function Tools(props: { text: VibeTranslate }) {
  const text = props.text
  return (
    <Section title={text('toolsTitle')}>
      <Paragraph>{text('toolsBody')}</Paragraph>
      <Paragraph>{text('toolsBody2')}</Paragraph>
      <p className='text-sm'>
        <NewTabLink href='/docs'>{text('toolsDocsLink')}</NewTabLink>
      </p>
      <div className='space-y-3'>
        <ToolRow
          tool='claude-code'
          name={text('toolClaudeName')}
          body={text('toolClaudeBody')}
        >
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
        <ToolRow
          tool='cline'
          name={text('toolClineName')}
          body={text('toolClineBody')}
        />
        <ToolRow
          tool='kilo'
          name={text('toolKiloName')}
          body={text('toolKiloBody')}
        />
        <ToolRow
          tool='opencode'
          name={text('toolOpencodeName')}
          body={text('toolOpencodeBody')}
        />
        <ToolRow
          tool='aider'
          name={text('toolAiderName')}
          body={text('toolAiderBody')}
        >
          <CodeBlock code={AIDER_COMMAND} language='bash'>
            <CodeBlockCopyButton />
          </CodeBlock>
        </ToolRow>
        <ToolRow
          tool='cursor'
          name={text('toolCursorName')}
          body={text('toolCursorBody')}
        />
      </div>
      <Paragraph>{text('toolRouteNote')}</Paragraph>
    </Section>
  )
}

/** Short pointer to the beginner route on `/docs` (the prompt that sets up the tool). */
export function AiHelp(props: { text: VibeTranslate }) {
  const text = props.text
  return (
    <div className='border-primary/40 bg-primary/5 space-y-2 rounded-xl border p-5'>
      <h2 className='flex items-center gap-2 text-lg font-semibold'>
        <Sparkles className='text-primary size-5 shrink-0' aria-hidden='true' />
        {text('aiHelpTitle')}
      </h2>
      <Paragraph>{text('aiHelpBody')}</Paragraph>
      <p className='text-sm font-medium'>
        <NewTabLink href={`/docs#${AI_SETUP_ANCHOR}`}>
          {text('aiHelpLink')}
        </NewTabLink>
      </p>
    </div>
  )
}

export function Fit(props: { text: VibeTranslate }) {
  const text = props.text
  return (
    <Section title={text('fitTitle')}>
      <Paragraph>{text('fitIntro')}</Paragraph>
      <div className='grid gap-3 sm:grid-cols-2'>
        <div className='space-y-2 rounded-lg border p-4'>
          <h3 className='text-base font-semibold'>{text('fitUsesTitle')}</h3>
          <BulletList text={text} keys={USE_KEYS} />
        </div>
        <div className='space-y-2 rounded-lg border p-4'>
          <h3 className='text-base font-semibold'>{text('fitNoTitle')}</h3>
          <BulletList text={text} keys={['fitNo1', 'fitNo2']} />
        </div>
      </div>
      <h3 className='pt-2 text-base font-semibold'>
        {text('fitPlatformsTitle')}
      </h3>
      <Paragraph>{text('fitPlatforms')}</Paragraph>
      <Paragraph>{text('fitPlatforms2')}</Paragraph>
      <Paragraph>{text('fitPlatforms3')}</Paragraph>
      <p className='text-sm'>
        <NewTabLink href='/docs'>{text('fitDocs')}</NewTabLink>
      </p>
    </Section>
  )
}
