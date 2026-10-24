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
import { create } from 'zustand'

import type { HwBonusApiConfig, HwBonusRule } from '@/features/hw/lib/hw-bonus'

/**
 * What the server said about the top-up bonus and the credit validity: the
 * public `/api/status` fields (`status`) and the signed-in customer's
 * `bonus_rules` from the packages answer (`packageRules`). Plain store, no
 * react-query, like the currency store, so text hooks can read it without a
 * query client.
 */
type HwBonusState = {
  status: HwBonusApiConfig
  packageRules: HwBonusRule[] | null
  setStatus: (status: HwBonusApiConfig) => void
  setPackageRules: (rules: HwBonusRule[] | null) => void
}

export const useHwBonusStore = create<HwBonusState>((set) => ({
  status: { rules: null, validityDays: null },
  packageRules: null,
  setStatus: (status) => set({ status }),
  setPackageRules: (packageRules) => set({ packageRules }),
}))
