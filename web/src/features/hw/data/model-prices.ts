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

/**
 * Price table of the HisarWeb AI channel-1 models, measured on 2026-10-22.
 * HisarWeb bills every model at half the official list price
 * (`HwListPriceDiscountPercent` in `service/hw_pricing.go`).
 *
 * Every amount is USD per 1 million tokens, copied from the verified
 * measurement (`prijzen.json`): the providers' own pricing pages, the
 * OpenRouter models list and HisarWeb's `/api/pricing`. A source that does not
 * list a model is `null`, and the table then shows a dash instead of a guess.
 * Lira amounts are computed from `MODEL_PRICES_TRY_PER_USD`, nothing here is
 * stored in lira.
 */

export type PriceProvider = 'Anthropic' | 'OpenAI' | 'Google'

export type PriceSourceId =
  | 'anthropic'
  | 'openai'
  | 'google'
  | 'openrouter'
  | 'hisarweb'

/** USD per 1 million tokens. */
export type TokenPrice = {
  input: number
  output: number
}

export type ModelPrice = {
  model: string
  provider: PriceProvider
  official: TokenPrice | null
  openrouter: TokenPrice | null
  hisarweb: TokenPrice | null
}

export type PriceSource = {
  id: PriceSourceId
  name: string
  url: string
  measuredAt: string
}

export const MODEL_PRICES_MEASURED_AT = '2026-10-22'

/** TRY per 1 USD (TCMB, without KDV): `custom_currency_exchange_rate`. */
export const MODEL_PRICES_TRY_PER_USD = 49.578
export const MODEL_PRICES_FX_DATE = '2026-10-20'

export const PRICE_SOURCES: readonly PriceSource[] = [
  {
    id: 'anthropic',
    name: 'Anthropic',
    url: 'https://platform.claude.com/docs/en/about-claude/pricing',
    measuredAt: MODEL_PRICES_MEASURED_AT,
  },
  {
    id: 'openai',
    name: 'OpenAI',
    url: 'https://developers.openai.com/api/docs/pricing',
    measuredAt: MODEL_PRICES_MEASURED_AT,
  },
  {
    id: 'google',
    name: 'Google',
    url: 'https://ai.google.dev/gemini-api/docs/pricing',
    measuredAt: MODEL_PRICES_MEASURED_AT,
  },
  {
    id: 'openrouter',
    name: 'OpenRouter',
    url: 'https://openrouter.ai/api/v1/models',
    measuredAt: MODEL_PRICES_MEASURED_AT,
  },
  {
    id: 'hisarweb',
    name: 'HisarWeb AI',
    url: 'https://api.hisarweb.be/api/pricing',
    measuredAt: MODEL_PRICES_MEASURED_AT,
  },
]

export const MODEL_PRICES: readonly ModelPrice[] = [
  {
    model: 'claude-opus-5-5',
    provider: 'Anthropic',
    official: { input: 4, output: 20 },
    openrouter: { input: 4, output: 20 },
    hisarweb: { input: 2, output: 10 },
  },
  {
    model: 'claude-sonnet-5-5',
    provider: 'Anthropic',
    official: { input: 2, output: 10 },
    openrouter: { input: 2, output: 10 },
    hisarweb: { input: 1, output: 5 },
  },
  {
    model: 'claude-haiku-5-5',
    provider: 'Anthropic',
    official: { input: 0.1, output: 0.5 },
    openrouter: { input: 0.1, output: 0.5 },
    hisarweb: { input: 0.05, output: 0.25 },
  },
  {
    model: 'claude-sonnet-5',
    provider: 'Anthropic',
    official: { input: 2, output: 10 },
    openrouter: { input: 2, output: 10 },
    hisarweb: { input: 1, output: 5 },
  },
  {
    model: 'claude-haiku-4-5',
    provider: 'Anthropic',
    official: { input: 1, output: 5 },
    openrouter: { input: 1, output: 5 },
    hisarweb: { input: 0.5, output: 2.5 },
  },
  {
    model: 'claude-fable-5-1',
    provider: 'Anthropic',
    official: { input: 10, output: 50 },
    openrouter: { input: 10, output: 50 },
    hisarweb: { input: 5, output: 25 },
  },
  {
    model: 'gpt-6-astra',
    provider: 'OpenAI',
    official: { input: 10, output: 50 },
    openrouter: { input: 10, output: 50 },
    hisarweb: { input: 5, output: 25 },
  },
  {
    model: 'gpt-6.1-sol',
    provider: 'OpenAI',
    official: { input: 2, output: 10 },
    openrouter: { input: 2, output: 10 },
    hisarweb: { input: 1, output: 5 },
  },
  {
    model: 'gpt-6-luna',
    provider: 'OpenAI',
    official: { input: 0.1, output: 0.5 },
    openrouter: { input: 0.1, output: 0.5 },
    hisarweb: { input: 0.05, output: 0.25 },
  },
  {
    // OpenRouter charges 10% above the official price for this model.
    model: 'gemini-3.8-flash',
    provider: 'Google',
    official: { input: 0.75, output: 3.75 },
    openrouter: { input: 0.825, output: 4.125 },
    hisarweb: { input: 0.375, output: 1.875 },
  },
]

/** Price of 1M input tokens plus 1M output tokens. */
export function blendedPrice(price: TokenPrice): number {
  return price.input + price.output
}

/**
 * Saving of `ours` against `reference` as a fraction (0.25 is 25% cheaper). It
 * is negative when `ours` is more expensive, and `null` when either side is
 * missing or the reference is free, so nothing is guessed.
 */
export function savingFraction(
  reference: TokenPrice | null,
  ours: TokenPrice | null
): number | null {
  if (!reference || !ours) return null
  const base = blendedPrice(reference)
  if (base <= 0) return null
  return (base - blendedPrice(ours)) / base
}
