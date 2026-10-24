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
import { Ban, CircleCheck, Lock } from 'lucide-react'
/** Step 6 of `/docs`: pick a program and connect it to HisarWeb AI. */
import { useEffect, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  findGuide,
  FREE_TRIAL_MODELS_TEXT,
  GUIDE_UI,
  KEY_TEST_STEPS,
  PROTOCOL_ROWS,
  RECOMMENDED_TOOLS,
  TOOL_GUIDES,
  TOOL_ORDER,
  type GuideLang,
  type GuideStatus,
  type GuideToolId,
  type GuideUi,
  type GuideVariant,
  type ToolGuide,
} from '../docs-tool-guides'
import { fillHwText, useHwLanguage } from '../texts'
import {
  GuideProblems,
  GuideShots,
  GuideSteps,
  GuideWorks,
  TAB_LABEL_CLASS,
  TAB_LIST_CLASS,
  TAB_TRIGGER_CLASS,
} from './docs-tool-guide-steps'

const PICKER_ID = 'hw-tool-picker'

/** `/docs#rehber-<tool id>` opens the page with that program's tab chosen. */
export const TOOL_GUIDE_HASH_PREFIX = 'rehber-'

function toolFromHash(): GuideToolId | undefined {
  const hash = decodeURIComponent(window.location.hash.slice(1))
  if (!hash.startsWith(TOOL_GUIDE_HASH_PREFIX)) return undefined
  const id = hash.slice(TOOL_GUIDE_HASH_PREFIX.length)
  return isToolId(id) ? id : undefined
}

const JUMP_BUTTON_CLASS = 'h-auto min-h-7 py-1 text-start whitespace-normal'

function isToolId(value: unknown): value is GuideToolId {
  return TOOL_ORDER.some((id) => id === value)
}

function scrollToPicker(): void {
  const element = document.querySelector(`[id="${PICKER_ID}"]`)
  if (element && typeof element.scrollIntoView === 'function') {
    element.scrollIntoView({ block: 'start' })
  }
}

function StatusIcon(props: { status: GuideStatus }) {
  const className = 'size-3.5 max-sm:hidden'
  switch (props.status) {
    case 'works':
      return <CircleCheck className={className} />
    case 'needs-paid-plan':
      return <Lock className={className} />
    default:
      return <Ban className={className} />
  }
}

function toolLabel(guide: ToolGuide, lang: GuideLang): string {
  return guide.tabLabel ? guide.tabLabel[lang] : guide.name
}

/** Buttons that jump to the tab of another program. */
function JumpButtons(props: {
  ids: readonly GuideToolId[]
  ui: GuideUi
  lang: GuideLang
  onSelect: (id: GuideToolId) => void
}) {
  return (
    <div className='flex flex-wrap gap-2'>
      {props.ids.map((id) => {
        const target = findGuide(id)
        if (!target) return null
        return (
          <Button
            key={id}
            type='button'
            variant='outline'
            size='sm'
            className={JUMP_BUTTON_CLASS}
            onClick={() => props.onSelect(id)}
          >
            {fillHwText(props.ui.tryInstead, { name: target.name })}
          </Button>
        )
      })}
    </div>
  )
}

function Recommendation(props: {
  ui: GuideUi
  lang: GuideLang
  onSelect: (id: GuideToolId) => void
}) {
  return (
    <div role='note' className='bg-muted/50 space-y-2 rounded-lg border p-3'>
      <p className='font-medium'>{props.ui.recommendation}</p>
      <p className='text-muted-foreground'>{props.ui.recommendationHint}</p>
      <JumpButtons
        ids={RECOMMENDED_TOOLS}
        ui={props.ui}
        lang={props.lang}
        onSelect={props.onSelect}
      />
    </div>
  )
}

function programsOf(
  protocol: ToolGuide['protocol'],
  ui: GuideUi,
  lang: GuideLang
) {
  return TOOL_GUIDES.filter((guide) => guide.protocol === protocol)
    .map((guide) => {
      const name = toolLabel(guide, lang)
      if (guide.status === 'needs-paid-plan') {
        return `${name} (${ui.paidPlanSuffix})`
      }
      return name
    })
    .join(', ')
}

function ProtocolTable(props: { ui: GuideUi; lang: GuideLang }) {
  const ui = props.ui
  return (
    <section className='space-y-3'>
      <h4 className='font-semibold'>{ui.protocolTitle}</h4>
      <p className='text-muted-foreground'>{ui.protocolIntro}</p>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className='whitespace-normal'>
              {ui.protocolColumnProtocol}
            </TableHead>
            <TableHead className='whitespace-normal'>
              {ui.protocolColumnUrl}
            </TableHead>
            <TableHead className='whitespace-normal'>
              {ui.protocolColumnPrograms}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {PROTOCOL_ROWS.map((row) => (
            <TableRow key={row.protocol}>
              <TableCell className='align-top whitespace-normal'>
                {ui.protocolNames[row.protocol]}
              </TableCell>
              <TableCell className='align-top whitespace-normal'>
                <code className='font-mono break-all'>{row.url}</code>
                {row.protocol === 'anthropic' ? (
                  <span className='text-muted-foreground mt-1 block text-xs font-normal'>
                    {ui.protocolNoSlash}
                  </span>
                ) : null}
              </TableCell>
              <TableCell className='align-top whitespace-normal'>
                {programsOf(row.protocol, ui, props.lang)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <p className='text-muted-foreground'>
        {fillHwText(ui.freeTrialNote, { models: FREE_TRIAL_MODELS_TEXT })}
      </p>
    </section>
  )
}

function KeyTest(props: { ui: GuideUi; lang: GuideLang }) {
  return (
    <section className='space-y-3'>
      <h4 className='font-semibold'>{props.ui.keyTestTitle}</h4>
      <p className='text-muted-foreground'>{props.ui.keyTestIntro}</p>
      <div
        role='note'
        className='border-warning/40 bg-warning/10 rounded-lg border p-3'
      >
        {props.ui.keyTestWarning}
      </div>
      <GuideSteps steps={KEY_TEST_STEPS} lang={props.lang} ui={props.ui} />
      <p className='text-muted-foreground'>{props.ui.keyTest401}</p>
    </section>
  )
}

function VariantBody(props: {
  variant: GuideVariant
  lang: GuideLang
  ui: GuideUi
}) {
  const variant = props.variant
  return (
    <div className='space-y-3'>
      {variant.intro ? (
        <p className='text-muted-foreground'>{variant.intro[props.lang]}</p>
      ) : null}
      <GuideSteps steps={variant.steps} lang={props.lang} ui={props.ui} />
    </div>
  )
}

/** One variant is shown directly; several variants get their own sub tabs. */
function Variants(props: {
  variants: readonly GuideVariant[]
  lang: GuideLang
  ui: GuideUi
}) {
  const first = props.variants[0]
  if (!first) return null
  if (props.variants.length === 1) {
    return <VariantBody variant={first} lang={props.lang} ui={props.ui} />
  }
  return (
    <Tabs defaultValue={first.id}>
      <TabsList aria-label={props.ui.variantLabel} className={TAB_LIST_CLASS}>
        {props.variants.map((variant) => (
          <TabsTrigger
            key={variant.id}
            value={variant.id}
            className={TAB_TRIGGER_CLASS}
          >
            <span className={TAB_LABEL_CLASS}>{variant.label[props.lang]}</span>
          </TabsTrigger>
        ))}
      </TabsList>
      {props.variants.map((variant) => (
        <TabsContent key={variant.id} value={variant.id} className='pt-3'>
          <VariantBody variant={variant} lang={props.lang} ui={props.ui} />
        </TabsContent>
      ))}
    </Tabs>
  )
}

function Limit(props: {
  guide: ToolGuide
  ui: GuideUi
  lang: GuideLang
  onSelect: (id: GuideToolId) => void
}) {
  const guide = props.guide
  if (!guide.limit) return null
  if (guide.status === 'works') return null
  const alternatives = guide.alternatives ?? []
  return (
    <div role='note' className='bg-muted/50 space-y-3 rounded-lg border p-3'>
      <p className='font-semibold'>{props.ui.limitTitle[guide.status]}</p>
      <p>{guide.limit[props.lang]}</p>
      {alternatives.length > 0 ? (
        <div className='space-y-2'>
          <p className='font-medium'>{props.ui.alternativesLead}</p>
          <JumpButtons
            ids={alternatives}
            ui={props.ui}
            lang={props.lang}
            onSelect={props.onSelect}
          />
        </div>
      ) : null}
    </div>
  )
}

function ProtocolLine(props: { guide: ToolGuide; ui: GuideUi }) {
  const guide = props.guide
  const row = PROTOCOL_ROWS.find((item) => item.protocol === guide.protocol)
  return (
    <p className='text-muted-foreground'>
      <span className='text-foreground font-medium'>
        {props.ui.protocolLine}:{' '}
      </span>
      {props.ui.protocolValues[guide.protocol]}
      {row ? (
        <>
          {' '}
          <code className='font-mono break-all'>{row.url}</code>
        </>
      ) : null}
    </p>
  )
}

function ToolPanel(props: {
  guide: ToolGuide
  ui: GuideUi
  lang: GuideLang
  onSelect: (id: GuideToolId) => void
}) {
  const guide = props.guide
  return (
    <div className='space-y-4 pt-3'>
      <h5 className='text-base font-semibold'>{guide.name}</h5>
      <p className='text-muted-foreground'>{guide.summary[props.lang]}</p>
      <ProtocolLine guide={guide} ui={props.ui} />
      <p className='text-muted-foreground'>
        <span className='text-foreground font-medium'>
          {props.ui.testedWith}:{' '}
        </span>
        {guide.testedWith[props.lang]}
      </p>
      <Limit
        guide={guide}
        ui={props.ui}
        lang={props.lang}
        onSelect={props.onSelect}
      />
      {guide.notice ? (
        <p className='text-muted-foreground'>{guide.notice[props.lang]}</p>
      ) : null}
      <Variants variants={guide.variants} lang={props.lang} ui={props.ui} />
      <GuideWorks works={guide.works} lang={props.lang} ui={props.ui} />
      <GuideProblems
        problems={guide.problems}
        lang={props.lang}
        ui={props.ui}
      />
      <GuideShots shots={guide.shots} lang={props.lang} />
    </div>
  )
}

/** Section of step 6: protocols, key test, one tab per program. */
export function ToolGuides() {
  const lang = useHwLanguage()
  const ui = GUIDE_UI[lang]
  const [tool, setTool] = useState<GuideToolId>(
    () => toolFromHash() ?? 'claude-code'
  )

  useEffect(() => {
    if (toolFromHash()) scrollToPicker()
  }, [])

  function select(id: GuideToolId) {
    setTool(id)
    scrollToPicker()
  }

  return (
    <div className='space-y-8 text-sm'>
      <p className='text-muted-foreground'>{ui.intro}</p>
      <Recommendation ui={ui} lang={lang} onSelect={select} />
      <ProtocolTable ui={ui} lang={lang} />
      <KeyTest ui={ui} lang={lang} />
      <section id={PICKER_ID} className='scroll-mt-24 space-y-3'>
        <h4 className='font-semibold'>{ui.pickTitle}</h4>
        <Tabs
          value={tool}
          onValueChange={(value) => {
            if (isToolId(value)) setTool(value)
          }}
        >
          <TabsList aria-label={ui.pickLabel} className={TAB_LIST_CLASS}>
            {TOOL_GUIDES.map((guide) => (
              <TabsTrigger
                key={guide.id}
                value={guide.id}
                className={TAB_TRIGGER_CLASS}
              >
                <StatusIcon status={guide.status} />
                <span className={TAB_LABEL_CLASS}>
                  {toolLabel(guide, lang)}
                </span>
              </TabsTrigger>
            ))}
          </TabsList>
          {TOOL_GUIDES.map((guide) => (
            <TabsContent key={guide.id} value={guide.id}>
              <ToolPanel guide={guide} ui={ui} lang={lang} onSelect={select} />
            </TabsContent>
          ))}
        </Tabs>
      </section>
      <div role='note' className='text-muted-foreground space-y-2'>
        <p>{ui.rooNote}</p>
        <JumpButtons ids={['cline']} ui={ui} lang={lang} onSelect={select} />
      </div>
    </div>
  )
}
