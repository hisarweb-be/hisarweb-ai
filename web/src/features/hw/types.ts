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

/** A money amount as the API sends it: decimal text, never a JSON number. */
export type HwMoneyText = string

/** Answer envelope shared by the public HisarWeb AI endpoints. */
export type HwApiEnvelope<T> = {
  success: boolean
  message?: string
  data: T
}

/** `GET /api/hw/pricing/compare` for one model and one monthly volume. */
export type HwPriceComparison = {
  model: string
  input_tokens: number
  output_tokens: number
  official_cost_usd: HwMoneyText
  official_cost_try: HwMoneyText
  first_purchase_try: HwMoneyText
  repeat_purchase_try: HwMoneyText
  savings_first_try: HwMoneyText
  savings_repeat_try: HwMoneyText
  try_per_usd: HwMoneyText
  price_source: string
  price_fetched_at: string
  fx_source: string
  fx_date: string
  stale: boolean
  prices_include_kdv: boolean
}

/** One task of one model inside the public quality summary. */
export type HwQualityTask = {
  task_id: string
  language: string
  kind: string
  score: number
  tokens_in: number
  tokens_out: number
  latency_ms: number
}

/** Per-model row of the public quality summary. */
export type HwQualityModel = {
  model: string
  reference: string
  avg_score: number
  avg_latency_ms: number
  tasks: HwQualityTask[]
}

/** `GET /api/hw/quality/latest`: the newest measurement, empty without one. */
export type HwQualityLatest = {
  measured_at: number
  file: string
  taskset_version: string
  task_count: number
  judge_model: string
  models: HwQualityModel[]
  note: string
  note_en: string
}
