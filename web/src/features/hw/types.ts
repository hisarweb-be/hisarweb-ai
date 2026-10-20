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
import type { HwLanguage } from './texts'
/** Shapes of the HisarWeb AI top-up routes (`router/hw-revolut.go`). */


/** One credit package as `GET /api/user/hw/packages` reports it. */
export interface HwCreditPackage {
  /** `purchase`, `free`, `affiliate` or `admin`. */
  source: string
  granted_quota: number
  remaining_quota: number
  /** Unix seconds. */
  created_at: number
  /** Unix seconds; the balance is spent before this moment. */
  expires_at: number
}

/** Answer of `GET /api/user/hw/packages`. */
export interface HwCreditPackages {
  packages: HwCreditPackage[]
  total_remaining_quota: number
  /** Quota units per US dollar. */
  quota_per_unit: number
  /** TRY per USD from the last price snapshot; empty when none exists yet. */
  try_per_usd: string
  /** Date of the rate (`YYYY-MM-DD`); empty when no snapshot exists yet. */
  fx_date: string
  fx_source: string
}

/**
 * Body of `POST /api/user/hw/orders`. `package_try` is the list value in whole
 * lira; the server computes the charged amount and the credit.
 */
export interface HwTopUpRequest {
  package_try: number
  language: HwLanguage
}

/** Answer of `POST /api/user/hw/orders`. */
export interface HwTopUpOrder {
  merchant_ref: string
  checkout_url: string
}

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

/** The three public state names of `GET /api/hw/status`. */
export type HwStatusState = 'operational' | 'degraded' | 'outage'

/** One own model in the public status answer. */
export type HwStatusModel = {
  model: string
  status: HwStatusState
  /**
   * Percentage over `availability_days`, rounded by the server. The server
   * sends 0 when there is no measurement yet, so 0 is shown as "no measurement".
   */
  availability_percent: number
  message_tr: string
  message_en: string
}

/** One outage in the public status answer. Times are unix seconds. */
export type HwStatusIncident = {
  started_at: number
  /** 0 while the outage still runs. */
  ended_at: number
  /** Length in seconds; 0 while the outage still runs. */
  duration: number
  message_tr: string
  message_en: string
}

/** `GET /api/hw/status`: the public service status, no login required. */
export type HwStatusAnswer = {
  /** Moment the answer was built, in unix seconds. */
  checked_at: number
  /** Window the availability figure covers, in days. */
  availability_days: number
  overall: HwStatusState
  models: HwStatusModel[]
  /** The running outage, when there is one. */
  incident?: HwStatusIncident | null
  /** Closed outages, newest first. */
  recent: HwStatusIncident[]
}
