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
import { CreditCard, Gift, Share2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { useHwCurrency } from '@/features/hw/hooks/use-hw-currency'
import { formatHwAmount } from '@/features/hw/lib/format'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuota } from '@/lib/format'

import { AFFILIATE_PERCENT, affiliateExample } from '../lib/affiliate'
import type { UserWalletData } from '../types'

interface AffiliateRewardsCardProps {
  user: UserWalletData | null
  affiliateLink: string
  onTransfer: () => void
  complianceConfirmed?: boolean
  loading?: boolean
}

export function AffiliateRewardsCard({
  user,
  affiliateLink,
  onTransfer,
  complianceConfirmed = true,
  loading,
}: AffiliateRewardsCardProps) {
  const { t, i18n } = useTranslation()
  const { currency, rates } = useHwCurrency()
  if (loading) {
    return (
      <Card data-card-hover='false' className='bg-muted/20 py-0'>
        <CardContent className='grid gap-4 p-3 sm:p-4 lg:grid-cols-[minmax(220px,1fr)_minmax(220px,0.72fr)_minmax(320px,1.15fr)] lg:items-center'>
          <div>
            <Skeleton className='h-5 w-32' />
            <Skeleton className='mt-2 h-4 w-48' />
          </div>
          <Skeleton className='h-14 rounded-lg' />
          <Skeleton className='h-10 rounded-lg' />
        </CardContent>
      </Card>
    )
  }

  const hasRewards = (user?.aff_quota ?? 0) > 0
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const example = affiliateExample(currency, rates)
  const steps = [
    {
      icon: Share2,
      title: t('Share your link'),
      text: t('Copy your personal link below and send it to a friend.'),
    },
    {
      icon: CreditCard,
      title: t('Your friend pays'),
      text: t('Your friend signs up through your link and tops up credit.'),
    },
    {
      icon: Gift,
      title: t('You get {{percent}}% as credit', {
        percent: AFFILIATE_PERCENT,
      }),
      text: t(
        'If your friend buys {{paid}}, you get {{reward}} as credit on your balance.',
        {
          paid: formatHwAmount(example.paid, example.currency, locale),
          reward: formatHwAmount(example.reward, example.currency, locale),
        }
      ),
    },
  ]

  return (
    <Card data-card-hover='false' className='bg-muted/20 py-0'>
      <CardContent className='grid gap-3 p-3 sm:gap-4 sm:p-4 lg:grid-cols-[minmax(200px,0.65fr)_minmax(280px,1fr)] lg:items-center'>
        <div className='flex min-w-0 items-center gap-2.5 lg:col-span-2'>
          <IconBadge tone='chart-3'>
            <Share2 />
          </IconBadge>
          <div className='min-w-0'>
            <h3 className='text-sm font-semibold'>{t('Referral Program')}</h3>
            <p className='text-muted-foreground text-xs'>
              {t('Three steps to earn credit with your personal link.')}
            </p>
          </div>
        </div>

        <ol className='grid gap-2 sm:grid-cols-3 lg:col-span-2'>
          {steps.map((step, index) => (
            <li
              key={step.title}
              className='bg-background/70 animate-in fade-in slide-in-from-bottom-2 fill-mode-both flex gap-2.5 rounded-lg border p-3 motion-reduce:animate-none'
              style={{ animationDelay: `${index * 120}ms` }}
            >
              <span
                className={`bg-primary/10 text-primary flex size-8 shrink-0 items-center justify-center rounded-full ${
                  index === steps.length - 1 ? 'motion-safe:animate-pulse' : ''
                }`}
              >
                <step.icon className='size-4' />
              </span>
              <div className='min-w-0'>
                <div className='text-sm font-semibold'>
                  {index + 1}. {step.title}
                </div>
                <p className='text-muted-foreground mt-0.5 text-xs'>
                  {step.text}
                </p>
              </div>
            </li>
          ))}
        </ol>

        <div className='grid grid-cols-3 gap-1.5 text-center'>
          {[
            [t('Pending'), formatQuota(user?.aff_quota ?? 0)],
            [t('Total Earned'), formatQuota(user?.aff_history_quota ?? 0)],
            [t('Invites'), String(user?.aff_count ?? 0)],
          ].map(([label, value]) => (
            <div key={label}>
              <div className='text-muted-foreground text-[10px] font-medium tracking-wider uppercase'>
                {label}
              </div>
              <div className='mt-0.5 text-sm font-semibold tabular-nums'>
                {value}
              </div>
            </div>
          ))}
        </div>

        <div className='flex items-center gap-2'>
          <Input
            value={affiliateLink}
            readOnly
            aria-label={t('Share your link')}
            className='border-muted bg-background/70 h-9 min-w-0 flex-1 font-mono text-xs'
          />
          <CopyButton
            value={affiliateLink}
            variant='outline'
            className='bg-background size-9 shrink-0'
            iconClassName='size-4'
            tooltip={t('Copy referral link')}
            aria-label={t('Copy referral link')}
          />
          {hasRewards && (
            <Button
              onClick={onTransfer}
              disabled={!complianceConfirmed}
              className='h-9 shrink-0 px-3'
              size='sm'
            >
              {t('Transfer to Balance')}
            </Button>
          )}
        </div>
        {!complianceConfirmed ? (
          <p className='text-muted-foreground text-xs lg:col-span-2'>
            {t(
              'Referral reward transfer is disabled until the administrator confirms compliance terms.'
            )}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}
