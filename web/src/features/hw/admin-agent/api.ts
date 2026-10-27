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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export type AgentAttachment = { name: string; mime: string; data: string }

export type AgentTurn = {
  role: 'user' | 'assistant'
  content: string
  attachments?: AgentAttachment[]
}

export type AssistantFaq = { id: string; question: string; answer: string }

export type AssistantAsk =
  | { matched: false }
  | { matched: true; id: string; answer: string }

/** The assistant answers in one of these languages; other UI languages get English. */
export function assistantLanguage(language: string | undefined): string {
  const base = (language ?? 'tr').slice(0, 2).toLowerCase()
  return base === 'tr' || base === 'nl' ? base : 'en'
}

export async function fetchAssistantFaq(lang: string): Promise<AssistantFaq[]> {
  const response = await api.get('/api/hw/assistant/faq', { params: { lang } })
  return (requireServerSuccess(response.data).data as { faq: AssistantFaq[] })
    .faq
}

/** A clicked chip: counts the hit; the admin variant answers in Altuner's voice. */
export async function askAssistantChip(
  id: string,
  lang: string,
  admin: boolean
): Promise<AssistantAsk> {
  const url = admin ? '/api/hw/admin/agent/ask' : '/api/hw/assistant/ask'
  const response = await api.post(url, { id, lang })
  return requireServerSuccess(response.data).data as AssistantAsk
}

const MAX_CUSTOMER_TURNS = 12

/** Customers send text only, the last 12 turns. */
export function customerTurns(
  messages: AgentTurn[]
): Pick<AgentTurn, 'role' | 'content'>[] {
  return messages
    .slice(-MAX_CUSTOMER_TURNS)
    .map(({ role, content }) => ({ role, content }))
}

/** Free typed text from a customer or a visitor goes to the model. */
export async function askAssistantChat(
  messages: AgentTurn[],
  lang: string
): Promise<string> {
  const response = await api.post('/api/hw/assistant/chat', {
    messages: customerTurns(messages),
    lang,
  })
  return (requireServerSuccess(response.data).data as { reply: string }).reply
}

export type AnswerFeedback = {
  question: string
  answer: string
  rating: 1 | -1
  lang: string
  correction?: string
}

export async function sendAnswerFeedback(
  feedback: AnswerFeedback,
  admin: boolean
): Promise<void> {
  const url = admin
    ? '/api/hw/admin/agent/feedback'
    : '/api/hw/assistant/feedback'
  requireServerSuccess((await api.post(url, feedback)).data)
}

export type AgentTextFix = { lang: string; shown: string; proposed: string }

export type AgentEscalation = { title: string; summary: string }

export type AgentAnswer = {
  reply: string
  text_fix?: AgentTextFix | null
  escalate?: AgentEscalation | null
}

export async function askAdminAgent(
  messages: AgentTurn[],
  lang: string
): Promise<AgentAnswer> {
  const response = await api.post('/api/hw/admin/agent/chat', {
    messages,
    lang,
  })
  return requireServerSuccess(response.data).data as AgentAnswer
}

export async function applyTextOverride(
  lang: string,
  key: string,
  value: string
): Promise<void> {
  const response = await api.post('/api/hw/admin/agent/texts', {
    lang,
    key,
    value,
  })
  requireServerSuccess(response.data)
}

export async function revertTextOverride(
  lang: string,
  key: string
): Promise<void> {
  const response = await api.post('/api/hw/admin/agent/texts/revert', {
    lang,
    key,
  })
  requireServerSuccess(response.data)
}

export async function escalateToBureau(
  escalation: AgentEscalation,
  page: string
): Promise<string> {
  const response = await api.post('/api/hw/admin/agent/escalate', {
    title: escalation.title,
    summary: escalation.summary,
    page,
  })
  const data = requireServerSuccess(response.data).data as { card?: string }
  return data?.card ?? ''
}
