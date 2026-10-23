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
/** Building blocks of the program guides: steps, code blocks, OS tabs, lists. */
import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  OS_ORDER,
  type GuideBlock,
  type GuideLang,
  type GuideOs,
  type GuideProblem,
  type GuideShot,
  type GuideStep,
  type GuideUi,
  type Localized,
} from '../docs-tool-guides'

/** Tab titles wrap on a phone instead of being cut off (same as commit 8f316ff). */
export const TAB_LIST_CLASS =
  'h-auto max-w-full flex-wrap justify-start gap-1 group-data-horizontal/tabs:h-auto'
export const TAB_TRIGGER_CLASS =
  'h-auto min-h-8 max-w-full flex-none gap-1.5 rounded-md px-2 py-1 text-xs whitespace-normal sm:px-3 sm:text-sm'
export const TAB_LABEL_CLASS = 'text-center leading-tight break-words'

interface LangProps {
  readonly lang: GuideLang
  readonly ui: GuideUi
}

/** One config block with its copy button. Long lines scroll sideways. */
export function GuideCode(props: { block: GuideBlock; lang: GuideLang }) {
  return (
    <div data-guide-block={props.block.kind} className='min-w-0'>
      {props.block.caption ? (
        <p className='text-muted-foreground text-xs'>
          {props.block.caption[props.lang]}
        </p>
      ) : null}
      <CodeBlock
        code={props.block.code}
        language={props.block.language}
        enableCollapse={false}
      >
        <CodeBlockCopyButton />
      </CodeBlock>
    </div>
  )
}

function GuideShotImage(props: { shot: GuideShot; lang: GuideLang }) {
  return (
    <img
      src={props.shot.src}
      alt={props.shot.alt[props.lang]}
      width={props.shot.width}
      height={props.shot.height}
      loading='lazy'
      style={{ maxWidth: props.shot.width }}
      className='h-auto w-full rounded-lg border'
    />
  )
}

/** Screenshots; renders nothing when the data has none. */
export function GuideShots(
  props: { shots?: readonly GuideShot[] } & Pick<LangProps, 'lang'>
) {
  if (!props.shots || props.shots.length === 0) return null
  return (
    <div className='space-y-2'>
      {props.shots.map((shot) => (
        <GuideShotImage key={shot.src} shot={shot} lang={props.lang} />
      ))}
    </div>
  )
}

function Labelled(props: { label: string; text: Localized; lang: GuideLang }) {
  return (
    <p className='text-muted-foreground'>
      <span className='text-foreground font-medium'>{props.label}: </span>
      {props.text[props.lang]}
    </p>
  )
}

function StepItem(props: { step: GuideStep } & LangProps) {
  const step = props.step
  return (
    <li className='space-y-2'>
      <p>{step.text[props.lang]}</p>
      {step.unverified ? (
        <p className='border-warning/40 bg-warning/10 rounded-md border px-2 py-1 text-xs'>
          {props.ui.unverified}
        </p>
      ) : null}
      {step.blocks?.map((block) => (
        <GuideCode key={block.code} block={block} lang={props.lang} />
      ))}
      {step.see ? (
        <Labelled label={props.ui.see} text={step.see} lang={props.lang} />
      ) : null}
      {step.note ? (
        <Labelled label={props.ui.note} text={step.note} lang={props.lang} />
      ) : null}
      <GuideShots shots={step.shots} lang={props.lang} />
    </li>
  )
}

function stepsForOs(
  steps: readonly GuideStep[],
  os: GuideOs
): readonly GuideStep[] {
  return steps.filter((step) => !step.only || step.only.includes(os))
}

function StepList(props: { steps: readonly GuideStep[] } & LangProps) {
  return (
    <ol className='list-decimal space-y-4 pl-5 marker:font-medium'>
      {props.steps.map((step) => (
        <StepItem
          key={step.text.en}
          step={step}
          lang={props.lang}
          ui={props.ui}
        />
      ))}
    </ol>
  )
}

/**
 * The numbered steps. Windows / macOS / Linux sub tabs appear only when a step
 * is limited to some systems with `only`; Windows opens first.
 */
export function GuideSteps(props: { steps: readonly GuideStep[] } & LangProps) {
  const hasOsSteps = props.steps.some((step) => step.only)
  if (!hasOsSteps) {
    return <StepList steps={props.steps} lang={props.lang} ui={props.ui} />
  }
  return (
    <Tabs defaultValue='windows'>
      <TabsList aria-label={props.ui.osLabel} className={TAB_LIST_CLASS}>
        {OS_ORDER.map((os) => (
          <TabsTrigger key={os} value={os} className={TAB_TRIGGER_CLASS}>
            <span className={TAB_LABEL_CLASS}>{props.ui.osNames[os]}</span>
          </TabsTrigger>
        ))}
      </TabsList>
      {OS_ORDER.map((os) => (
        <TabsContent key={os} value={os} className='pt-3'>
          <StepList
            steps={stepsForOs(props.steps, os)}
            lang={props.lang}
            ui={props.ui}
          />
        </TabsContent>
      ))}
    </Tabs>
  )
}

/** "How you know it works": the literal expected results. */
export function GuideWorks(props: { works: readonly Localized[] } & LangProps) {
  if (props.works.length === 0) return null
  return (
    <section className='space-y-2'>
      <h6 className='font-semibold'>{props.ui.worksTitle}</h6>
      <ul className='text-muted-foreground list-disc space-y-1 pl-5'>
        {props.works.map((item) => (
          <li key={item.en}>{item[props.lang]}</li>
        ))}
      </ul>
    </section>
  )
}

/** Message, cause and fix of the common problems. */
export function GuideProblems(
  props: { problems: readonly GuideProblem[] } & LangProps
) {
  if (props.problems.length === 0) return null
  return (
    <section className='space-y-2'>
      <h6 className='font-semibold'>{props.ui.problemsTitle}</h6>
      <ul className='space-y-3'>
        {props.problems.map((problem) => (
          <li
            key={problem.message.en}
            className='space-y-1 rounded-lg border p-3'
          >
            <p className='font-medium break-words'>
              {problem.message[props.lang]}
            </p>
            <Labelled
              label={props.ui.problemCause}
              text={problem.cause}
              lang={props.lang}
            />
            <Labelled
              label={props.ui.problemFix}
              text={problem.fix}
              lang={props.lang}
            />
          </li>
        ))}
      </ul>
    </section>
  )
}
