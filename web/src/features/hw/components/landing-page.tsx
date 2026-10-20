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
  ArrowRight,
  BadgePercent,
  CircleCheck,
  Clock,
  CreditCard,
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardHeader } from '@/components/ui/card'

import { useHwText } from '../texts'

/**
 * HisarWeb AI landing page.
 *
 * The comparison and quality pages are built by other runs, so the links to
 * them are plain anchors: they must work before those routes exist and without
 * a dependency on the generated route tree. `/sign-up` already exists, so that
 * one goes through `Link`.
 */
export function LandingPage() {
  const text = useHwText()

  const benefits = [
    {
      key: 'payment',
      title: text('benefitPaymentTitle'),
      body: text('benefitPaymentBody'),
      icon: CreditCard,
    },
    {
      key: 'discount',
      title: text('benefitDiscountTitle'),
      body: text('benefitDiscountBody'),
      icon: BadgePercent,
    },
    {
      key: 'expiry',
      title: text('benefitExpiryTitle'),
      body: text('benefitExpiryBody'),
      icon: Clock,
    },
  ]

  const steps = [
    { key: '1', title: text('step1Title'), body: text('step1Body') },
    { key: '2', title: text('step2Title'), body: text('step2Body') },
    { key: '3', title: text('step3Title'), body: text('step3Body') },
    { key: '4', title: text('step4Title'), body: text('step4Body') },
  ]

  const faq = [
    { key: '1', question: text('faq1Q'), answer: text('faq1A') },
    { key: '2', question: text('faq2Q'), answer: text('faq2A') },
    { key: '3', question: text('faq3Q'), answer: text('faq3A') },
    { key: '4', question: text('faq4Q'), answer: text('faq4A') },
    { key: '5', question: text('faq5Q'), answer: text('faq5A') },
  ]

  return (
    <main>
      <section className='border-border/40 border-b px-4 pt-24 pb-14 sm:px-6 md:pt-32 md:pb-20'>
        <div className='mx-auto max-w-3xl text-center'>
          <p className='border-primary/25 bg-primary/5 text-primary inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium'>
            {text('landingBadge')}
          </p>
          <h1 className='mt-6 text-[clamp(1.75rem,5vw,3rem)] leading-tight font-bold tracking-tight'>
            {text('landingTitle')}
          </h1>
          <p className='text-muted-foreground mx-auto mt-5 max-w-2xl text-base leading-relaxed md:text-lg'>
            {text('landingLead')}
          </p>
          <div className='mt-8 flex flex-col items-center justify-center gap-3 sm:flex-row'>
            <Button
              size='lg'
              className='h-11 w-full rounded-lg px-5 text-sm sm:w-auto'
              render={<Link to='/sign-up' />}
            >
              {text('landingCtaPrimary')}
              <ArrowRight className='ml-1 size-4' aria-hidden='true' />
            </Button>
            <Button
              size='lg'
              variant='outline'
              className='h-11 w-full rounded-lg px-5 text-sm sm:w-auto'
              render={<a href='/fiyat-karsilastirma' />}
            >
              {text('landingCtaSecondary')}
            </Button>
          </div>
          <p className='text-muted-foreground mx-auto mt-6 max-w-xl text-xs leading-relaxed'>
            {text('landingCatalogNote')}
          </p>
        </div>
      </section>

      <section
        aria-labelledby='hw-benefits-title'
        className='border-border/40 border-b px-4 py-14 sm:px-6 md:py-20'
      >
        <div className='mx-auto max-w-6xl'>
          <h2
            id='hw-benefits-title'
            className='text-center text-2xl font-bold tracking-tight md:text-3xl'
          >
            {text('benefitTitle')}
          </h2>
          <ul className='mt-8 grid gap-4 md:grid-cols-3 md:gap-6'>
            {benefits.map((benefit) => (
              <li key={benefit.key}>
                <Card className='h-full'>
                  <CardHeader>
                    <span className='bg-primary/10 text-primary flex size-10 items-center justify-center rounded-lg'>
                      <benefit.icon className='size-5' aria-hidden='true' />
                    </span>
                    <h3 className='text-base leading-snug font-medium'>
                      {benefit.title}
                    </h3>
                    <p className='text-muted-foreground text-sm leading-relaxed'>
                      {benefit.body}
                    </p>
                  </CardHeader>
                </Card>
              </li>
            ))}
          </ul>
        </div>
      </section>

      <section
        aria-labelledby='hw-trial-title'
        className='border-border/40 border-b px-4 py-14 sm:px-6 md:py-20'
      >
        <div className='bg-accent/40 border-border/50 mx-auto flex max-w-3xl flex-col gap-5 rounded-xl border p-6 md:flex-row md:items-center md:justify-between'>
          <div>
            <h2
              id='hw-trial-title'
              className='text-xl font-semibold tracking-tight md:text-2xl'
            >
              {text('trialTitle')}
            </h2>
            <p className='text-muted-foreground mt-2 text-sm leading-relaxed'>
              {text('trialBody')}
            </p>
          </div>
          <Button
            className='h-10 shrink-0 rounded-lg px-4'
            render={<Link to='/sign-up' />}
          >
            {text('trialCta')}
          </Button>
        </div>
      </section>

      <section
        aria-labelledby='hw-steps-title'
        className='border-border/40 border-b px-4 py-14 sm:px-6 md:py-20'
      >
        <div className='mx-auto max-w-6xl'>
          <h2
            id='hw-steps-title'
            className='text-center text-2xl font-bold tracking-tight md:text-3xl'
          >
            {text('stepsTitle')}
          </h2>
          <ol className='mt-8 grid gap-4 sm:grid-cols-2 md:gap-6 lg:grid-cols-4'>
            {steps.map((step) => (
              <li
                key={step.key}
                className='border-border/50 bg-muted/20 flex gap-3 rounded-xl border p-4'
              >
                <span
                  aria-hidden='true'
                  className='bg-foreground text-background flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-bold'
                >
                  {step.key}
                </span>
                <div>
                  <h3 className='text-sm font-semibold'>{step.title}</h3>
                  <p className='text-muted-foreground mt-1 text-sm leading-relaxed'>
                    {step.body}
                  </p>
                </div>
              </li>
            ))}
          </ol>
        </div>
      </section>

      <section
        aria-labelledby='hw-compare-title'
        className='border-border/40 border-b px-4 py-14 sm:px-6 md:py-20'
      >
        <div className='mx-auto grid max-w-6xl items-center gap-6 md:grid-cols-2 md:gap-10'>
          <div>
            <h2
              id='hw-compare-title'
              className='text-2xl font-bold tracking-tight md:text-3xl'
            >
              {text('compareTeaserTitle')}
            </h2>
            <p className='text-muted-foreground mt-3 text-sm leading-relaxed md:text-base'>
              {text('compareTeaserBody')}
            </p>
            <p className='text-muted-foreground mt-3 text-xs'>
              {text('compareKdv')}
            </p>
            <div className='mt-6 flex flex-col gap-3 sm:flex-row sm:items-center'>
              <Button
                className='h-10 rounded-lg px-4'
                render={<a href='/fiyat-karsilastirma' />}
              >
                {text('compareTeaserCta')}
                <ArrowRight className='ml-1 size-4' aria-hidden='true' />
              </Button>
              <a
                href='/kalite'
                className='text-primary inline-flex items-center gap-1 text-sm font-medium underline-offset-4 hover:underline'
              >
                {text('qualityTeaserLink')}
              </a>
            </div>
          </div>
          <ul className='border-border/50 bg-muted/20 space-y-3 rounded-xl border p-5'>
            {[
              text('compareOfficial'),
              text('comparePriceDate'),
              text('compareFxDate'),
            ].map((line) => (
              <li key={line} className='flex items-start gap-2 text-sm'>
                <CircleCheck
                  className='text-primary mt-0.5 size-4 shrink-0'
                  aria-hidden='true'
                />
                <span>{line}</span>
              </li>
            ))}
          </ul>
        </div>
      </section>

      <section
        aria-labelledby='hw-faq-title'
        className='border-border/40 border-b px-4 py-14 sm:px-6 md:py-20'
      >
        <div className='mx-auto max-w-3xl'>
          <h2
            id='hw-faq-title'
            className='text-center text-2xl font-bold tracking-tight md:text-3xl'
          >
            {text('faqTitle')}
          </h2>
          <div className='mt-8 space-y-3'>
            {faq.map((item) => (
              <details
                key={item.key}
                className='border-border/50 bg-card rounded-lg border px-4 py-3'
              >
                <summary className='cursor-pointer text-sm font-medium'>
                  {item.question}
                </summary>
                <p className='text-muted-foreground mt-2 text-sm leading-relaxed'>
                  {item.answer}
                </p>
              </details>
            ))}
          </div>
        </div>
      </section>

      <section
        aria-labelledby='hw-final-cta-title'
        className='px-4 py-14 sm:px-6 md:py-20'
      >
        <div className='mx-auto max-w-2xl text-center'>
          <h2
            id='hw-final-cta-title'
            className='text-2xl font-bold tracking-tight md:text-3xl'
          >
            {text('finalCtaTitle')}
          </h2>
          <p className='text-muted-foreground mx-auto mt-3 max-w-lg text-sm leading-relaxed md:text-base'>
            {text('finalCtaBody')}
          </p>
          <Button
            size='lg'
            className='mt-7 h-11 rounded-lg px-5 text-sm'
            render={<Link to='/sign-up' />}
          >
            {text('finalCtaButton')}
            <ArrowRight className='ml-1 size-4' aria-hidden='true' />
          </Button>
        </div>
      </section>
    </main>
  )
}
