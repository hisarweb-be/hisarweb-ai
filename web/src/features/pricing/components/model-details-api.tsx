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
import {
  ChevronRight,
  Gauge,
  KeyRound,
  PlugZap,
  ScrollText,
  Sigma,
  Zap,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'
import type { BundledLanguage } from 'shiki/bundle/web'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { CopyButton } from '@/components/copy-button'
import {
  StaticDataTable,
  staticDataTableClassNames as tableStyles,
} from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Hint } from '@/features/hw/components/hint'
import { useStatus } from '@/hooks/use-status'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { HIDDEN_CUSTOMER_GROUPS } from '../lib/customer-groups'
import { describeEndpoint } from '../lib/endpoint-labels'
import {
  buildRateLimits,
  buildSupportedParameters,
  type RateLimit,
  type SupportedParameter,
} from '../lib/mock-stats'
import { replaceModelInPath } from '../lib/model-helpers'
import { plainGroupLabel, plainParamDescriptionKey } from '../lib/plain-metrics'
import type { PricingModel } from '../types'

// ---------------------------------------------------------------------------
// Code-sample registry
// ---------------------------------------------------------------------------
//
// Each sample is keyed by language and endpoint type. The endpoint type comes
// from the model's `supported_endpoint_types`; we render samples only for the
// types the model actually supports. This keeps copy-pasted code accurate and
// provider-shaped (OpenAI vs Anthropic vs Gemini, etc.).

type Lang = 'curl' | 'python' | 'typescript' | 'javascript'

const LANG_LABELS: Record<Lang, string> = {
  curl: 'cURL',
  python: 'Python',
  typescript: 'TypeScript',
  javascript: 'JavaScript',
}

const LANG_HIGHLIGHT: Record<Lang, BundledLanguage> = {
  curl: 'bash',
  python: 'python',
  typescript: 'typescript',
  javascript: 'javascript',
}

/** The one key placeholder every sample and every sentence about it uses. */
const API_KEY_PLACEHOLDER = 'YOUR_API_KEY'

type SampleContext = {
  baseUrl: string
  apiKey: string
  modelName: string
  endpointType: string
  endpointPath: string
}

function buildChatSample(lang: Lang, ctx: SampleContext): string {
  const url = `${ctx.baseUrl}${ctx.endpointPath}`
  const isResponses = ctx.endpointType === 'openai-response'
  const isReasoning = /^o[1-4]|reasoning|thinking|deepseek-r/i.test(
    ctx.modelName
  )
  const userMessage = 'Explain quantum entanglement in one paragraph.'

  const bodyJson = isResponses
    ? JSON.stringify({ model: ctx.modelName, input: userMessage }, null, 2)
    : JSON.stringify(
        {
          model: ctx.modelName,
          messages: [{ role: 'user', content: userMessage }],
          ...(isReasoning ? {} : { temperature: 0.7 }),
        },
        null,
        2
      )

  const fnCall = isResponses ? 'responses.create' : 'chat.completions.create'

  if (lang === 'curl') {
    return [
      `curl ${url} \\`,
      `  -H "Authorization: Bearer ${ctx.apiKey}" \\`,
      `  -H "Content-Type: application/json" \\`,
      `  -d '${bodyJson.replace(/\n/g, '\n     ')}'`,
    ].join('\n')
  }

  if (lang === 'python') {
    return [
      'from openai import OpenAI',
      '',
      'client = OpenAI(',
      `    base_url="${ctx.baseUrl}/v1",`,
      `    api_key="${ctx.apiKey}",`,
      ')',
      '',
      isResponses
        ? `response = client.${fnCall}(\n    model="${ctx.modelName}",\n    input="${userMessage}",\n)\n\nprint(response.output_text)`
        : `completion = client.${fnCall}(\n    model="${ctx.modelName}",\n    messages=[\n        {"role": "user", "content": "${userMessage}"}\n    ],\n)\n\nprint(completion.choices[0].message.content)`,
    ].join('\n')
  }

  if (lang === 'typescript') {
    return [
      `import OpenAI from 'openai'`,
      '',
      `const client = new OpenAI({`,
      `  baseURL: '${ctx.baseUrl}/v1',`,
      `  apiKey: '${ctx.apiKey}',`,
      `})`,
      '',
      isResponses
        ? `const response = await client.${fnCall}({\n  model: '${ctx.modelName}',\n  input: '${userMessage}',\n})\n\nconsole.log(response.output_text)`
        : `const completion = await client.${fnCall}({\n  model: '${ctx.modelName}',\n  messages: [{ role: 'user', content: '${userMessage}' }],\n})\n\nconsole.log(completion.choices[0].message.content)`,
    ].join('\n')
  }

  return [
    `const response = await fetch('${url}', {`,
    `  method: 'POST',`,
    `  headers: {`,
    `    Authorization: 'Bearer ${ctx.apiKey}',`,
    `    'Content-Type': 'application/json',`,
    `  },`,
    `  body: JSON.stringify(${bodyJson}),`,
    `})`,
    '',
    `const data = await response.json()`,
    `console.log(data)`,
  ].join('\n')
}

function buildAnthropicSample(lang: Lang, ctx: SampleContext): string {
  const url = `${ctx.baseUrl}${ctx.endpointPath}`
  const userMessage = 'Explain quantum entanglement in one paragraph.'

  if (lang === 'curl') {
    const body = JSON.stringify(
      {
        model: ctx.modelName,
        max_tokens: 1024,
        messages: [{ role: 'user', content: userMessage }],
      },
      null,
      2
    )
    return [
      `curl ${url} \\`,
      `  -H "x-api-key: ${ctx.apiKey}" \\`,
      `  -H "anthropic-version: 2023-06-01" \\`,
      `  -H "Content-Type: application/json" \\`,
      `  -d '${body.replace(/\n/g, '\n     ')}'`,
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'import anthropic',
      '',
      'client = anthropic.Anthropic(',
      `    base_url="${ctx.baseUrl}",`,
      `    api_key="${ctx.apiKey}",`,
      ')',
      '',
      `message = client.messages.create(`,
      `    model="${ctx.modelName}",`,
      `    max_tokens=1024,`,
      `    messages=[{"role": "user", "content": "${userMessage}"}],`,
      ')',
      '',
      'print(message.content[0].text)',
    ].join('\n')
  }
  if (lang === 'typescript') {
    return [
      `import Anthropic from '@anthropic-ai/sdk'`,
      '',
      `const client = new Anthropic({`,
      `  baseURL: '${ctx.baseUrl}',`,
      `  apiKey: '${ctx.apiKey}',`,
      `})`,
      '',
      `const message = await client.messages.create({`,
      `  model: '${ctx.modelName}',`,
      `  max_tokens: 1024,`,
      `  messages: [{ role: 'user', content: '${userMessage}' }],`,
      `})`,
      '',
      `console.log(message.content[0].text)`,
    ].join('\n')
  }
  return [
    `const response = await fetch('${url}', {`,
    `  method: 'POST',`,
    `  headers: {`,
    `    'x-api-key': '${ctx.apiKey}',`,
    `    'anthropic-version': '2023-06-01',`,
    `    'Content-Type': 'application/json',`,
    `  },`,
    `  body: JSON.stringify({`,
    `    model: '${ctx.modelName}',`,
    `    max_tokens: 1024,`,
    `    messages: [{ role: 'user', content: '${userMessage}' }],`,
    `  }),`,
    `})`,
    '',
    `const data = await response.json()`,
    `console.log(data.content[0].text)`,
  ].join('\n')
}

function buildGeminiSample(lang: Lang, ctx: SampleContext): string {
  const url = `${ctx.baseUrl}${ctx.endpointPath}?key=${ctx.apiKey}`
  const userMessage = 'Explain quantum entanglement in one paragraph.'

  if (lang === 'curl') {
    const body = JSON.stringify(
      { contents: [{ parts: [{ text: userMessage }] }] },
      null,
      2
    )
    return [
      `curl '${url}' \\`,
      `  -H 'Content-Type: application/json' \\`,
      `  -d '${body.replace(/\n/g, '\n     ')}'`,
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'import google.generativeai as genai',
      '',
      `genai.configure(api_key="${ctx.apiKey}")`,
      '',
      `model = genai.GenerativeModel("${ctx.modelName}")`,
      `response = model.generate_content("${userMessage}")`,
      '',
      `print(response.text)`,
    ].join('\n')
  }
  if (lang === 'typescript') {
    return [
      `import { GoogleGenerativeAI } from '@google/generative-ai'`,
      '',
      `const genAI = new GoogleGenerativeAI('${ctx.apiKey}')`,
      `const model = genAI.getGenerativeModel({ model: '${ctx.modelName}' })`,
      '',
      `const result = await model.generateContent('${userMessage}')`,
      `console.log(result.response.text())`,
    ].join('\n')
  }
  return [
    `const response = await fetch('${url}', {`,
    `  method: 'POST',`,
    `  headers: { 'Content-Type': 'application/json' },`,
    `  body: JSON.stringify({`,
    `    contents: [{ parts: [{ text: '${userMessage}' }] }],`,
    `  }),`,
    `})`,
    '',
    `const data = await response.json()`,
    `console.log(data.candidates[0].content.parts[0].text)`,
  ].join('\n')
}

function buildEmbeddingSample(lang: Lang, ctx: SampleContext): string {
  const url = `${ctx.baseUrl}${ctx.endpointPath}`
  const text = 'The food was delicious and the waiter…'

  if (lang === 'curl') {
    const body = JSON.stringify({ model: ctx.modelName, input: text }, null, 2)
    return [
      `curl ${url} \\`,
      `  -H "Authorization: Bearer ${ctx.apiKey}" \\`,
      `  -H "Content-Type: application/json" \\`,
      `  -d '${body.replace(/\n/g, '\n     ')}'`,
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'from openai import OpenAI',
      '',
      `client = OpenAI(base_url="${ctx.baseUrl}/v1", api_key="${ctx.apiKey}")`,
      '',
      'response = client.embeddings.create(',
      `    model="${ctx.modelName}",`,
      `    input="${text}",`,
      ')',
      '',
      'print(response.data[0].embedding[:8])',
    ].join('\n')
  }
  if (lang === 'typescript') {
    return [
      `import OpenAI from 'openai'`,
      '',
      `const client = new OpenAI({`,
      `  baseURL: '${ctx.baseUrl}/v1',`,
      `  apiKey: '${ctx.apiKey}',`,
      `})`,
      '',
      `const response = await client.embeddings.create({`,
      `  model: '${ctx.modelName}',`,
      `  input: '${text}',`,
      `})`,
      '',
      `console.log(response.data[0].embedding.slice(0, 8))`,
    ].join('\n')
  }
  return [
    `const response = await fetch('${url}', {`,
    `  method: 'POST',`,
    `  headers: {`,
    `    Authorization: 'Bearer ${ctx.apiKey}',`,
    `    'Content-Type': 'application/json',`,
    `  },`,
    `  body: JSON.stringify({`,
    `    model: '${ctx.modelName}',`,
    `    input: '${text}',`,
    `  }),`,
    `})`,
    '',
    `const data = await response.json()`,
    `console.log(data.data[0].embedding.slice(0, 8))`,
  ].join('\n')
}

function buildImageSample(lang: Lang, ctx: SampleContext): string {
  const url = `${ctx.baseUrl}${ctx.endpointPath}`
  const prompt = 'A serene koi pond at sunset, ukiyo-e style.'

  if (lang === 'curl') {
    const body = JSON.stringify(
      { model: ctx.modelName, prompt, size: '1024x1024', n: 1 },
      null,
      2
    )
    return [
      `curl ${url} \\`,
      `  -H "Authorization: Bearer ${ctx.apiKey}" \\`,
      `  -H "Content-Type: application/json" \\`,
      `  -d '${body.replace(/\n/g, '\n     ')}'`,
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'from openai import OpenAI',
      '',
      `client = OpenAI(base_url="${ctx.baseUrl}/v1", api_key="${ctx.apiKey}")`,
      '',
      'response = client.images.generate(',
      `    model="${ctx.modelName}",`,
      `    prompt="${prompt}",`,
      `    size="1024x1024",`,
      `    n=1,`,
      ')',
      '',
      'print(response.data[0].url)',
    ].join('\n')
  }
  if (lang === 'typescript') {
    return [
      `import OpenAI from 'openai'`,
      '',
      `const client = new OpenAI({`,
      `  baseURL: '${ctx.baseUrl}/v1',`,
      `  apiKey: '${ctx.apiKey}',`,
      `})`,
      '',
      `const response = await client.images.generate({`,
      `  model: '${ctx.modelName}',`,
      `  prompt: '${prompt}',`,
      `  size: '1024x1024',`,
      `  n: 1,`,
      `})`,
      '',
      `console.log(response.data[0].url)`,
    ].join('\n')
  }
  return [
    `const response = await fetch('${url}', {`,
    `  method: 'POST',`,
    `  headers: {`,
    `    Authorization: 'Bearer ${ctx.apiKey}',`,
    `    'Content-Type': 'application/json',`,
    `  },`,
    `  body: JSON.stringify({`,
    `    model: '${ctx.modelName}',`,
    `    prompt: '${prompt}',`,
    `    size: '1024x1024',`,
    `    n: 1,`,
    `  }),`,
    `})`,
    '',
    `const data = await response.json()`,
    `console.log(data.data[0].url)`,
  ].join('\n')
}

function buildSample(
  lang: Lang,
  endpointType: string,
  ctx: SampleContext
): string {
  if (endpointType === 'anthropic') return buildAnthropicSample(lang, ctx)
  if (endpointType === 'gemini') return buildGeminiSample(lang, ctx)
  if (endpointType === 'embeddings' || endpointType === 'jina-rerank')
    return buildEmbeddingSample(lang, ctx)
  if (endpointType === 'image-generation') return buildImageSample(lang, ctx)
  return buildChatSample(lang, ctx)
}

// ---------------------------------------------------------------------------
// How do I connect
// ---------------------------------------------------------------------------

function ConnectStep(props: { number: number; children: React.ReactNode }) {
  return (
    <li className='flex items-start gap-3'>
      <span className='bg-primary/10 text-primary flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold'>
        {props.number}
      </span>
      <div className='min-w-0 flex-1 space-y-1.5 pt-0.5 text-sm'>
        {props.children}
      </div>
    </li>
  )
}

function ConnectSection(props: { connectUrl: string }) {
  const { t } = useTranslation()
  return (
    <section>
      <SectionTitle icon={PlugZap}>{t('How do I connect')}</SectionTitle>
      <ol className='border-border/60 bg-muted/20 space-y-4 rounded-lg border p-4'>
        <ConnectStep number={1}>
          <p>
            {t(
              'Create an API key. It works like a password that your tool uses to talk to the model.'
            )}
          </p>
          <Button size='sm' variant='outline' render={<Link to='/keys' />}>
            {t('Open the API keys page')}
          </Button>
        </ConnectStep>
        <ConnectStep number={2}>
          <p>
            {t(
              'Copy this address. Some tools call it the base URL or the endpoint.'
            )}
          </p>
          <div className='flex items-center gap-1'>
            <code className='bg-muted min-w-0 truncate rounded px-2 py-1 font-mono text-xs'>
              {props.connectUrl}
            </code>
            <CopyButton
              value={props.connectUrl}
              size='icon'
              variant='ghost'
              className='size-7'
              tooltip={t('Copy the address')}
            />
          </div>
        </ConnectStep>
        <ConnectStep number={3}>
          <p>
            {t(
              'Paste the key and the address into your tool, for example Cursor, Cline or Claude Code, and pick this model.'
            )}
          </p>
        </ConnectStep>
      </ol>
    </section>
  )
}

// ---------------------------------------------------------------------------
// Code samples section
// ---------------------------------------------------------------------------

function CodeSamplesSection(props: {
  model: PricingModel
  endpointMap: Record<string, { path?: string; method?: string }>
}) {
  const { t } = useTranslation()
  const { status } = useStatus()

  const baseUrl = useMemo(() => {
    const candidate =
      (status as Record<string, unknown> | null)?.server_address ??
      (status as Record<string, unknown> | null)?.serverAddress ??
      (status?.data as Record<string, unknown> | undefined)?.server_address ??
      (status?.data as Record<string, unknown> | undefined)?.serverAddress
    if (candidate && typeof candidate === 'string') {
      return candidate.replace(/\/$/, '')
    }
    if (typeof window !== 'undefined') return window.location.origin
    return 'https://api.example.com'
  }, [status])

  const endpoints = useMemo(() => {
    const types = props.model.supported_endpoint_types || []
    return types
      .map((type) => {
        const info = props.endpointMap[type] || {}
        let path = info.path || ''
        if (path && path.includes('{model}')) {
          path = replaceModelInPath(path, props.model.model_name || '')
        }
        return { type, path, method: info.method || 'POST' }
      })
      .filter((e) => Boolean(e.path))
  }, [props.model, props.endpointMap])

  const [endpointType, setEndpointType] = useState<string>(
    endpoints[0]?.type ?? ''
  )
  const [lang, setLang] = useState<Lang>('curl')

  const activeEndpoint = useMemo(() => {
    return endpoints.find((e) => e.type === endpointType) ?? endpoints[0]
  }, [endpointType, endpoints])

  if (endpoints.length === 0 || !activeEndpoint) {
    return null
  }

  const code = buildSample(lang, activeEndpoint.type, {
    baseUrl,
    apiKey: API_KEY_PLACEHOLDER,
    modelName: props.model.model_name || '',
    endpointType: activeEndpoint.type,
    endpointPath: activeEndpoint.path,
  })
  // The tools a beginner uses (Cursor, Cline) want the address with /v1;
  // Anthropic-compatible tools (Claude Code) want the bare address.
  const connectUrl =
    activeEndpoint.type === 'anthropic' ? baseUrl : `${baseUrl}/v1`

  return (
    <>
      <ConnectSection connectUrl={connectUrl} />
      <section>
        <SectionTitle icon={ScrollText}>{t('Code samples')}</SectionTitle>
        <p className='text-muted-foreground mb-3 text-xs'>
          {t(
            'Only needed if you write code yourself. Tools like Cursor, Cline and Claude Code just need the three steps above.'
          )}
        </p>

        <div className='flex flex-wrap items-center gap-2'>
          {endpoints.length > 1 && (
            <Tabs value={endpointType} onValueChange={setEndpointType}>
              <TabsList className='bg-muted/40 h-auto flex-wrap p-0.5'>
                {endpoints.map((ep) => (
                  <TabsTrigger
                    key={ep.type}
                    value={ep.type}
                    className='h-auto min-h-7 px-2.5 text-xs whitespace-normal'
                  >
                    {describeEndpoint(ep.type, t).label}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          )}

          <Tabs
            value={lang}
            onValueChange={(v) => setLang(v as Lang)}
            className='ml-auto'
          >
            <TabsList className='bg-muted/40 h-8 p-0.5'>
              {(Object.keys(LANG_LABELS) as Lang[]).map((l) => (
                <TabsTrigger key={l} value={l} className='h-7 px-2.5 text-xs'>
                  {LANG_LABELS[l]}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        </div>

        <div className='mt-3'>
          <CodeBlock code={code} language={LANG_HIGHLIGHT[lang]}>
            <CodeBlockCopyButton />
          </CodeBlock>
        </div>

        <p className='text-muted-foreground mt-2 text-xs'>
          <Trans
            t={t}
            i18nKey='In the examples, <code>YOUR_API_KEY</code> stands for your own API key from step 1.'
            components={{
              code: (
                <code className='bg-muted rounded px-1 py-0.5 font-mono text-[11px]' />
              ),
            }}
          />
        </p>
      </section>
    </>
  )
}

// ---------------------------------------------------------------------------
// Supported parameters table
// ---------------------------------------------------------------------------

function SupportedParametersSection(props: { model: PricingModel }) {
  const { t } = useTranslation()
  const params = useMemo(
    () => buildSupportedParameters(props.model),
    [props.model]
  )

  if (params.length === 0) return null

  return (
    <section>
      <SectionTitle icon={Sigma}>{t('Supported parameters')}</SectionTitle>
      <p className='text-muted-foreground mb-3 text-xs'>
        {t(
          'Settings you can add to your request. Only the ones marked required must be sent; the rest are optional.'
        )}
      </p>
      <StaticDataTable
        className={tableStyles.sectionContainer}
        headerRowClassName={tableStyles.mutedHeaderRow}
        data={params}
        getRowKey={(param) => param.name}
        getRowClassName={() => 'hover:bg-muted/20'}
        columns={[
          {
            id: 'parameter',
            header: (
              <Hint
                hint={t(
                  'The name of the setting, as you write it in your request.'
                )}
              >
                {t('Parameter')}
              </Hint>
            ),
            className: 'h-9 w-44',
            cellClassName: tableStyles.topCell,
            cell: (p) => (
              <div className='flex items-center gap-1.5'>
                <code className='font-mono text-sm font-medium'>{p.name}</code>
                {p.required && (
                  <Badge
                    variant='outline'
                    className='h-6 border-rose-500/40 px-2 text-sm text-rose-600 dark:text-rose-400'
                  >
                    {t('required')}
                  </Badge>
                )}
              </div>
            ),
          },
          {
            id: 'type',
            header: (
              <Hint
                hint={t(
                  'What kind of value the setting takes: a number, true or false, text, or a list.'
                )}
              >
                {t('Type')}
              </Hint>
            ),
            className: 'h-9 w-24',
            cellClassName: tableStyles.topCell,
            cell: (p) => (
              <Badge
                variant='secondary'
                className='h-7 rounded-full px-2.5 font-mono text-sm font-normal'
              >
                {p.type}
              </Badge>
            ),
          },
          {
            id: 'range',
            header: (
              <Hint
                hint={t(
                  'The value used when you leave the setting out, and the values that are allowed.'
                )}
              >
                {t('Default / range')}
              </Hint>
            ),
            className: 'h-9 w-32',
            cellClassName: tableStyles.topCell,
            cell: (p) => <ParamRangeCell param={p} />,
          },
          {
            id: 'description',
            header: t('What it does'),
            className: 'h-9',
            cellClassName: tableStyles.topMutedCell,
            cell: (p) => t(plainParamDescriptionKey(p.descriptionKey)),
          },
        ]}
      />
    </section>
  )
}

function ParamRangeCell(props: { param: SupportedParameter }) {
  const { defaultValue, range, enumValues } = props.param
  if (defaultValue !== undefined) {
    return (
      <div className='flex flex-wrap items-center gap-1'>
        <span className='text-muted-foreground text-sm'>=</span>
        <code className='bg-muted rounded px-1.5 py-0.5 font-mono text-sm'>
          {String(defaultValue)}
        </code>
        {range && (
          <span className='text-muted-foreground text-sm'>{range}</span>
        )}
      </div>
    )
  }
  if (range) {
    return (
      <span className='text-muted-foreground font-mono text-sm'>{range}</span>
    )
  }
  if (enumValues && enumValues.length > 0) {
    return (
      <div className='flex flex-wrap gap-0.5'>
        {enumValues.map((v) => (
          <code
            key={v}
            className='bg-muted text-muted-foreground rounded px-1.5 py-0.5 font-mono text-sm'
          >
            {v}
          </code>
        ))}
      </div>
    )
  }
  return <span className='text-muted-foreground text-sm'>—</span>
}

// ---------------------------------------------------------------------------
// Usage limits
// ---------------------------------------------------------------------------

function RateLimitSentences(props: { limit: RateLimit; locale?: string }) {
  const { t } = useTranslation()
  const { limit, locale } = props
  return (
    <ul className='list-inside list-disc space-y-0.5 text-sm'>
      {limit.rpm > 0 && (
        <li>
          {t('Up to {{count}} requests per minute', {
            count: formatNumber(limit.rpm, locale),
          })}
        </li>
      )}
      {limit.tpm > 0 && (
        <li>
          {t('Up to {{count}} tokens per minute', {
            count: formatNumber(limit.tpm, locale),
          })}
        </li>
      )}
      {limit.rpd > 0 && (
        <li>
          {t('Up to {{count}} requests per day', {
            count: formatNumber(limit.rpd, locale),
          })}
        </li>
      )}
    </ul>
  )
}

function RateLimitsSection(props: { model: PricingModel }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  // A customer cannot pick a group, so only the groups that are on sale count,
  // and groups that read the same ("Everyone") are shown once.
  const limits = useMemo(() => {
    const all = buildRateLimits(props.model)
    const sold = all.filter(
      (limit) => !HIDDEN_CUSTOMER_GROUPS.includes(limit.group)
    )
    const rows = sold.length > 0 ? sold : all.slice(0, 1)
    const seen = new Set<string>()
    return rows.filter((limit) => {
      const key = plainGroupLabel(limit.group, t)
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })
  }, [props.model, t])

  if (limits.length === 0) return null

  return (
    <section>
      <SectionTitle icon={Gauge}>
        <Hint
          hint={t(
            'How much you can send in a given time, so that nobody overloads the service.'
          )}
        >
          {t('Rate limits')}
        </Hint>
      </SectionTitle>
      <div className='border-border/60 bg-muted/20 space-y-3 rounded-lg border p-3'>
        {limits.map((limit) => (
          <div key={limit.group} className='space-y-1'>
            {limits.length > 1 && (
              <div className='text-muted-foreground text-xs font-medium'>
                {plainGroupLabel(limit.group, t)}
              </div>
            )}
            <RateLimitSentences limit={limit} locale={locale} />
          </div>
        ))}
      </div>
    </section>
  )
}

// ---------------------------------------------------------------------------
// Authentication preview
// ---------------------------------------------------------------------------

function AuthSection() {
  const { t } = useTranslation()
  const codeClass = 'bg-muted rounded px-1 py-0.5 font-mono text-[11px]'
  return (
    <section>
      <SectionTitle icon={KeyRound}>
        <Hint
          hint={t(
            'The proof that a request comes from you, so only you can use your credit.'
          )}
        >
          {t('Authentication')}
        </Hint>
      </SectionTitle>
      <div className='border-border/60 bg-muted/20 flex items-start gap-2 rounded-lg border p-3'>
        <ChevronRight className='text-muted-foreground mt-0.5 size-3.5 shrink-0' />
        <div className='space-y-1.5 text-xs leading-relaxed'>
          <p>
            {t(
              'Every request must prove it comes from you. Your tool does this for you once you paste your API key. If you write your own code, send the key in a header, an extra line of information that travels with your request, like this:'
            )}{' '}
            <code className={codeClass}>
              {`Authorization: Bearer ${API_KEY_PLACEHOLDER}`}
            </code>
          </p>
          <p>
            <Trans
              t={t}
              i18nKey='Anthropic-compatible connections (Claude Code) send it as <code>x-api-key: YOUR_API_KEY</code> instead.'
              components={{ code: <code className={codeClass} /> }}
            />
          </p>
          <p className='text-muted-foreground'>
            <Trans
              t={t}
              i18nKey='You can create and manage your keys on the <link>API keys page</link>.'
              components={{
                link: <Link to='/keys' className='underline' />,
              }}
            />
          </p>
        </div>
      </div>
    </section>
  )
}

// ---------------------------------------------------------------------------
// Composite API tab
// ---------------------------------------------------------------------------

export function ModelDetailsApi(props: {
  model: PricingModel
  endpointMap: Record<string, { path?: string; method?: string }>
}) {
  return (
    <div className='space-y-6'>
      <CodeSamplesSection model={props.model} endpointMap={props.endpointMap} />
      <AuthSection />
      <SupportedParametersSection model={props.model} />
      <RateLimitsSection model={props.model} />
    </div>
  )
}

// ---------------------------------------------------------------------------
// Local UI helpers
// ---------------------------------------------------------------------------

function SectionTitle(props: {
  children: React.ReactNode
  icon: React.ComponentType<{ className?: string }>
}) {
  const Icon = props.icon
  return (
    <h3 className='text-foreground mb-3 flex items-center gap-1.5 text-sm font-semibold'>
      <Icon className='text-muted-foreground size-3.5' />
      {props.children}
    </h3>
  )
}

// Re-export so the parent can keep its own SectionTitle if it wants:
export { Zap as ApiTabIcon }
