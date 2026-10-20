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
import { useMutation, useQueryClient } from '@tanstack/react-query'

import { createHwTopUpOrder } from '../api'

import { hwPackagesQueryKey } from './use-hw-packages'

/**
 * Starts a top-up order. The caller words the failure itself (the server's
 * answer is English), so the shared error toast is turned off here.
 */
export function useHwTopUpOrder() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: createHwTopUpOrder,
    meta: { errorToast: false },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: hwPackagesQueryKey })
    },
  })
}
