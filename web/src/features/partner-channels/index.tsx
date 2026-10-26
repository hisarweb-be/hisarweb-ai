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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { createServerError } from '@/lib/server-error-message'

import { grantPartnerChannel, listPartnerChannels } from './api'

const QUERY_KEY = ['partner-channels']

export function PartnerChannels() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [userId, setUserId] = useState('')

  const listQuery = useQuery({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const res = await listPartnerChannels()
      if (!res.success || !Array.isArray(res.data)) {
        throw createServerError(res, t('We could not load the channels.'))
      }
      return res.data
    },
    retry: false,
  })

  const grantMutation = useMutation({
    mutationFn: async (id: number) => {
      const res = await grantPartnerChannel(id)
      if (!res.success || !res.data) {
        throw createServerError(res, t('We could not grant the channel.'))
      }
      return res.data
    },
    onSuccess: (result) => {
      toast.success(
        result.bonus_pending
          ? t(
              'Channel credit granted. The bonus follows once the account is verified.'
            )
          : t('Channel granted.')
      )
      setUserId('')
      void queryClient.invalidateQueries({ queryKey: QUERY_KEY })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const parsedId = Number(userId)
  const validId = Number.isInteger(parsedId) && parsedId > 0
  const rows = listQuery.data ?? []

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Partner Channels')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <form
            className='flex flex-wrap items-end gap-2'
            onSubmit={(event) => {
              event.preventDefault()
              if (validId) grantMutation.mutate(parsedId)
            }}
          >
            <div className='space-y-1'>
              <label htmlFor='partner-channel-user-id' className='text-sm'>
                {t('User ID')}
              </label>
              <Input
                id='partner-channel-user-id'
                inputMode='numeric'
                value={userId}
                onChange={(event) => setUserId(event.target.value)}
                className='w-40'
              />
            </div>
            <Button
              type='submit'
              disabled={!validId || grantMutation.isPending}
            >
              {t('Grant channel')}
            </Button>
            <Button
              type='button'
              variant='outline'
              onClick={() => void listQuery.refetch()}
              disabled={listQuery.isFetching}
            >
              <RefreshCw data-icon='inline-start' aria-hidden='true' />
              {t('Refresh')}
            </Button>
          </form>

          {listQuery.isLoading && <Skeleton className='h-24 w-full' />}
          {listQuery.isError && (
            <ErrorState
              title={t('We could not load the channels.')}
              description={
                listQuery.error instanceof Error
                  ? listQuery.error.message
                  : undefined
              }
              onRetry={() => void listQuery.refetch()}
            />
          )}
          {listQuery.isSuccess && rows.length === 0 && (
            <p className='text-muted-foreground text-sm'>
              {t('No channels granted yet.')}
            </p>
          )}
          {listQuery.isSuccess && rows.length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('User ID')}</TableHead>
                  <TableHead>{t('Username')}</TableHead>
                  <TableHead>{t('Display Name')}</TableHead>
                  <TableHead>{t('Granted at')}</TableHead>
                  <TableHead>{t('Expires')}</TableHead>
                  <TableHead>{t('Signups')}</TableHead>
                  <TableHead>{t('Paying customers')}</TableHead>
                  <TableHead>{t('Earned credit')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((row) => (
                  <TableRow key={row.user_id}>
                    <TableCell>{row.user_id}</TableCell>
                    <TableCell>{row.username}</TableCell>
                    <TableCell>{row.display_name}</TableCell>
                    <TableCell>
                      {formatTimestampToDate(row.granted_at)}
                    </TableCell>
                    <TableCell>
                      {row.expires_at > 0
                        ? formatTimestampToDate(row.expires_at)
                        : '-'}
                    </TableCell>
                    <TableCell>{row.signups}</TableCell>
                    <TableCell>{row.paying_customers}</TableCell>
                    <TableCell>{formatQuota(row.earned_quota)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
