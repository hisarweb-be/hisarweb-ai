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
import type { GroupOption, ModelOption } from '../../types'

/** Best value first: the cheap default, then the everyday coding model. */
const PREFERRED_MODELS = ['claude-haiku-5-5', 'claude-sonnet-5-5']

/** Retired names (hwRetiredModels in service/hw_pricing.go) and the model that replaced them. */
const MODEL_SUCCESSORS: Readonly<Record<string, string>> = {
  'claude-haiku-4-5': 'claude-haiku-5-5',
  'claude-sonnet-5': 'claude-sonnet-5-5',
}

/**
 * Returns the model to switch to, or null to keep the current one. A saved
 * older model moves to its successor, and a missing model falls back to the
 * best-value model the user can reach.
 */
export function getModelFallback(
  models: ModelOption[],
  currentModel: string
): string | null {
  const has = (value: string | undefined) =>
    value !== undefined && models.some((model) => model.value === value)

  const successor = MODEL_SUCCESSORS[currentModel]
  if (has(successor)) {
    return successor
  }

  if (has(currentModel) || models.length === 0) {
    return null
  }

  return PREFERRED_MODELS.find(has) ?? models[0].value
}

export function shouldClearModelForGroup(
  models: ModelOption[],
  currentModel: string
): boolean {
  if (currentModel === '') {
    return false
  }

  return !models.some((model) => model.value === currentModel)
}

/**
 * Returns the group to switch to, or null to keep the current one. A missing
 * group falls back to the account's own group, so a paid customer never lands
 * in another group by accident, then to 'default', then to the first group.
 */
export function getGroupFallback(
  groups: GroupOption[],
  currentGroup: string,
  userGroup?: string
): string | null {
  const has = (value: string | undefined) =>
    value !== undefined && groups.some((group) => group.value === value)

  if (has(currentGroup) || groups.length === 0) {
    return null
  }

  return [userGroup, 'default'].find(has) ?? groups[0].value
}

export function getOptionLoadErrorMessage(
  error: unknown,
  fallbackMessage: string
): string {
  return error instanceof Error ? error.message : fallbackMessage
}
