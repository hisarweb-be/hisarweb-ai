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
  KeyRound,
  MailCheck,
  Send,
  ShieldCheck,
  UserPlus,
} from 'lucide-react'
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
import { useRef, type CSSProperties, type ReactNode } from 'react'

import { Button } from '@/components/ui/button'

import '@/styles/hw-landing.css'

import { useLandingMotion } from '../hooks/use-landing-motion'
import { useHwText } from '../texts'
import { useVibeText } from '../vibe-texts'

type Translate = ReturnType<typeof useHwText>

function stagger(index: number): CSSProperties {
  return { '--d': index } as CSSProperties
}

/** Splits the headline around the accent phrase so only that part shimmers. */
function Headline(props: { title: string; accent: string }): ReactNode {
  const index = props.title.indexOf(props.accent)
  if (index < 0) return props.title
  return (
    <>
      {props.title.slice(0, index)}
      <span className='hwl-accent'>{props.accent}</span>
      {props.title.slice(index + props.accent.length)}
    </>
  )
}

function Hero(props: { text: Translate }) {
  const text = props.text
  return (
    <section className='hwl-hero' data-glow>
      <div className='hwl-aurora' aria-hidden='true'>
        <span className='hwl-blob hwl-blob-1' />
        <span className='hwl-blob hwl-blob-2' />
        <span className='hwl-blob hwl-blob-3' />
      </div>
      <div className='hwl-grid' aria-hidden='true' />
      <div className='hwl-spot' aria-hidden='true' />
      <div className='hwl-grain' aria-hidden='true' />

      <p className='hwl-badge'>
        <span className='hwl-badge-dot' aria-hidden='true' />
        {text('landingBadge')}
      </p>
      <h1 className='hwl-h1'>
        <Headline
          title={text('landingTitle')}
          accent={text('landingTitleAccent')}
        />
      </h1>
      <p className='hwl-lead'>{text('landingLead')}</p>
      <div className='hwl-cta-row'>
        <Button
          size='lg'
          className='hwl-btn-glow h-12 w-full rounded-xl px-6 text-sm sm:w-auto'
          render={<Link to='/sign-up' />}
        >
          {text('landingCtaPrimary')}
          <ArrowRight className='ml-1 size-4' aria-hidden='true' />
        </Button>
        <Button
          size='lg'
          variant='outline'
          className='hwl-btn-ghost h-12 w-full rounded-xl px-6 text-sm sm:w-auto'
          render={<a href='/fiyat-karsilastirma' />}
        >
          {text('landingCtaSecondary')}
        </Button>
      </div>
      <p className='hwl-note'>{text('landingCatalogNote')}</p>

      <div className='hwl-stage' aria-hidden='true'>
        <span className='hwl-chip hwl-chip-1'>
          <i />
          {text('benefitPaymentTitle')}
        </span>
        <span className='hwl-chip hwl-chip-2'>
          <i />
          {text('trialFigure')}
        </span>
        <span className='hwl-chip hwl-chip-3'>
          <i />
          {text('benefitExpiryTitle')}
        </span>
        <div className='hwl-stage-card'>
          <div className='hwl-stage-bar'>
            <i />
            <i />
            <i />
            <span>api.hisarweb.be</span>
          </div>
          <pre className='hwl-term'>
            <span className='line' style={{ '--i': 0 } as CSSProperties}>
              <span className='p'>$</span> curl{' '}
              <span className='s'>
                https://api.hisarweb.be/v1/chat/completions
              </span>
            </span>
            <span className='line' style={{ '--i': 1 } as CSSProperties}>
              {'  '}-H{' '}
              <span className='s'>"Authorization: Bearer sk-••••••••"</span>
            </span>
            <span className='line' style={{ '--i': 2 } as CSSProperties}>
              {'  '}-d <span className='k'>{'{"messages": [ … ]}'}</span>
            </span>
            <span className='line' style={{ '--i': 3 } as CSSProperties}>
              <span className='ok'>✓ 200 OK</span>
              <span className='hwl-caret' />
            </span>
          </pre>
        </div>
      </div>
    </section>
  )
}

function Ticker(props: { items: string[] }) {
  const group = (hidden: boolean) => (
    <div className='hwl-ticker-group' aria-hidden={hidden || undefined}>
      {props.items.map((item) => (
        <span key={item} className='hwl-ticker-item'>
          {item}
        </span>
      ))}
    </div>
  )
  return (
    <div className='hwl-ticker' aria-hidden='true'>
      <div className='hwl-ticker-track'>
        {group(false)}
        {group(true)}
      </div>
    </div>
  )
}

function Film(props: { text: Translate }) {
  const text = props.text
  return (
    <section aria-labelledby='hw-film-title' className='hwl-section'>
      <div className='mx-auto max-w-5xl'>
        <h2 id='hw-film-title' className='hwl-h2 text-center' data-reveal>
          {text('filmTitle')}
        </h2>
        <p
          className='text-muted-foreground mx-auto mt-5 max-w-2xl text-center text-base leading-relaxed md:text-lg'
          data-reveal
          style={stagger(1)}
        >
          {text('filmBody')}
        </p>
        <div className='hwl-film' data-reveal style={stagger(2)}>
          <video
            controls
            playsInline
            preload='metadata'
            poster='/film/hisarweb-ai-lansman.jpg'
            src='/film/hisarweb-ai-lansman.mp4'
            aria-label={text('filmLabel')}
          />
        </div>
      </div>
    </section>
  )
}

function Benefits(props: { text: Translate }) {
  const text = props.text
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
  return (
    <section aria-labelledby='hw-benefits-title' className='hwl-section'>
      <div className='mx-auto max-w-6xl'>
        <h2 id='hw-benefits-title' className='hwl-h2 text-center' data-reveal>
          {text('benefitTitle')}
        </h2>
        <ul className='hwl-cards'>
          {benefits.map((benefit, index) => (
            <li key={benefit.key} data-reveal style={stagger(index)}>
              <div className='hwl-card' data-glow>
                <span className='hwl-icon'>
                  <benefit.icon className='size-6' aria-hidden='true' />
                </span>
                <h3>{benefit.title}</h3>
                <p>{benefit.body}</p>
              </div>
            </li>
          ))}
        </ul>
      </div>
    </section>
  )
}

function Vibe() {
  const text = useVibeText()
  return (
    <section aria-labelledby='hw-vibe-title' className='hwl-section'>
      <div className='mx-auto max-w-3xl text-center'>
        <h2 id='hw-vibe-title' className='hwl-h2' data-reveal>
          {text('landingTitle')}
        </h2>
        <p
          className='text-muted-foreground mx-auto mt-5 max-w-2xl text-base leading-relaxed md:text-lg'
          data-reveal
          style={stagger(1)}
        >
          {text('landingBody')}
        </p>
        <div className='mt-8' data-reveal style={stagger(2)}>
          <Button
            size='lg'
            variant='outline'
            render={<a href='/vibe-kodlama' />}
          >
            {text('landingButton')}
          </Button>
        </div>
      </div>
    </section>
  )
}

function Compare(props: { text: Translate }) {
  const text = props.text
  return (
    <section aria-labelledby='hw-compare-title' className='hwl-section'>
      <div className='hwl-compare mx-auto max-w-6xl'>
        <div data-reveal>
          <h2 id='hw-compare-title' className='hwl-h2'>
            {text('compareTeaserTitle')}
          </h2>
          <p className='text-muted-foreground mt-5 text-base leading-relaxed md:text-lg'>
            {text('compareTeaserBody')}
          </p>
          <p className='text-muted-foreground mt-3 text-xs'>
            {text('compareKdv')}
          </p>
          <div className='mt-8 flex flex-col gap-4 sm:flex-row sm:flex-wrap sm:items-center'>
            <Button
              className='hwl-btn-glow h-11 rounded-xl px-5'
              render={<a href='/fiyat-karsilastirma' />}
            >
              {text('compareTeaserCta')}
              <ArrowRight className='ml-1 size-4' aria-hidden='true' />
            </Button>
            <a href='/kalite' className='hwl-link'>
              {text('qualityTeaserLink')}
            </a>
            <a href='/durum' className='hwl-link'>
              {text('navStatus')}
            </a>
          </div>
        </div>

        <div className='hwl-panel' data-reveal style={stagger(1)}>
          <div className='hwl-big' aria-hidden='true'>
            <span className='n' />
            <span className='u'>%</span>
          </div>
          <div className='hwl-bars'>
            <div aria-hidden='true' className='hwl-cut' />
            <div>
              <div className='hwl-bar-label'>
                <span>{text('compareOfficial')}</span>
                <span aria-hidden='true'>100%</span>
              </div>
              <div className='hwl-track'>
                <div className='hwl-fill hwl-fill-official' />
              </div>
            </div>
            <div>
              <div className='hwl-bar-label'>
                <span>{text('compareOursRepeat')}</span>
                <span aria-hidden='true'>50%</span>
              </div>
              <div className='hwl-track'>
                <div className='hwl-fill hwl-fill-ours' />
              </div>
            </div>
          </div>
          <ul className='hwl-checks'>
            {[text('comparePriceDate'), text('compareFxDate')].map((line) => (
              <li key={line}>
                <CircleCheck
                  className='text-primary mt-0.5 size-4 shrink-0'
                  aria-hidden='true'
                />
                <span>{line}</span>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </section>
  )
}

function Trial(props: { text: Translate }) {
  const text = props.text
  return (
    <section aria-labelledby='hw-trial-title' className='px-4 py-6 sm:px-6'>
      <div className='mx-auto max-w-5xl' data-reveal>
        <div className='hwl-trial'>
          <div className='hwl-trial-fig' aria-hidden='true'>
            {text('trialFigure')}
          </div>
          <div>
            <h2
              id='hw-trial-title'
              className='text-2xl font-bold tracking-tight md:text-3xl'
            >
              {text('trialTitle')}
            </h2>
            <p className='text-muted-foreground mt-2 text-sm leading-relaxed md:text-base'>
              {text('trialBody')}
            </p>
          </div>
          <Button
            size='lg'
            className='hwl-btn-glow h-12 shrink-0 rounded-xl px-6'
            render={<Link to='/sign-up' />}
          >
            {text('trialCta')}
          </Button>
        </div>
      </div>
    </section>
  )
}

function Steps(props: { text: Translate }) {
  const text = props.text
  const steps = [
    {
      key: '1',
      title: text('step1Title'),
      body: text('step1Body'),
      icon: UserPlus,
    },
    {
      key: '2',
      title: text('step2Title'),
      body: text('step2Body'),
      icon: MailCheck,
    },
    {
      key: '3',
      title: text('step3Title'),
      body: text('step3Body'),
      icon: Send,
    },
    {
      key: '4',
      title: text('step4Title'),
      body: text('step4Body'),
      icon: KeyRound,
    },
  ]
  return (
    <section aria-labelledby='hw-steps-title' className='hwl-section'>
      <div className='mx-auto max-w-6xl'>
        <h2 id='hw-steps-title' className='hwl-h2 text-center' data-reveal>
          {text('stepsTitle')}
        </h2>
        <ol className='hwl-steps'>
          {steps.map((step, index) => (
            <li
              key={step.key}
              className='hwl-step'
              data-reveal
              style={stagger(index)}
            >
              <span className='hwl-node' aria-hidden='true'>
                <step.icon className='size-6' />
                <b>{step.key}</b>
              </span>
              <h3>{step.title}</h3>
              <p>{step.body}</p>
              {step.key === '4' ? (
                <div className='hwl-step-key' aria-hidden='true'>
                  sk-••••••••••••••••
                </div>
              ) : null}
            </li>
          ))}
        </ol>
        <div className='hwl-why' data-reveal>
          <span className='hwl-icon' aria-hidden='true'>
            <ShieldCheck className='size-5' />
          </span>
          <div>
            <h3>{text('stepsPhoneWhyTitle')}</h3>
            <p>{text('stepsPhoneWhyBody')}</p>
          </div>
        </div>
      </div>
    </section>
  )
}

function Faq(props: { text: Translate }) {
  const text = props.text
  const faq = [
    { key: '1', question: text('faq1Q'), answer: text('faq1A') },
    { key: '2', question: text('faq2Q'), answer: text('faq2A') },
    { key: '3', question: text('faq3Q'), answer: text('faq3A') },
    { key: '4', question: text('faq4Q'), answer: text('faq4A') },
    { key: '5', question: text('faq5Q'), answer: text('faq5A') },
  ]
  return (
    <section aria-labelledby='hw-faq-title' className='hwl-section'>
      <div className='mx-auto max-w-3xl'>
        <h2 id='hw-faq-title' className='hwl-h2 text-center' data-reveal>
          {text('faqTitle')}
        </h2>
        <div className='hwl-faq mt-12 space-y-3' data-reveal>
          {faq.map((item) => (
            <details key={item.key}>
              <summary>{item.question}</summary>
              <p>{item.answer}</p>
            </details>
          ))}
        </div>
      </div>
    </section>
  )
}

function FinalCta(props: { text: Translate }) {
  const text = props.text
  return (
    <section
      aria-labelledby='hw-final-cta-title'
      className='px-4 pb-20 sm:px-6 md:pb-32'
    >
      <div className='hwl-final mx-auto max-w-5xl' data-reveal>
        <div className='hwl-aurora' aria-hidden='true'>
          <span className='hwl-blob hwl-blob-1' />
          <span className='hwl-blob hwl-blob-2' />
        </div>
        <h2 id='hw-final-cta-title' className='hwl-h2 mx-auto max-w-2xl'>
          {text('finalCtaTitle')}
        </h2>
        <p className='text-muted-foreground mx-auto mt-5 max-w-lg text-base leading-relaxed md:text-lg'>
          {text('finalCtaBody')}
        </p>
        <Button
          size='lg'
          className='hwl-btn-glow mt-9 h-12 rounded-xl px-7 text-sm'
          render={<Link to='/sign-up' />}
        >
          {text('finalCtaButton')}
          <ArrowRight className='ml-1 size-4' aria-hidden='true' />
        </Button>
      </div>
    </section>
  )
}

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
  const root = useRef<HTMLElement>(null)
  useLandingMotion(root)

  return (
    <main ref={root} className='hwl'>
      <Hero text={text} />
      <Ticker
        items={[
          text('benefitPaymentTitle'),
          text('benefitDiscountTitle'),
          text('benefitExpiryTitle'),
          text('trialTitle'),
          text('step4Title'),
        ]}
      />
      <Film text={text} />
      <Benefits text={text} />
      <Vibe />
      <Compare text={text} />
      <Trial text={text} />
      <Steps text={text} />
      <Faq text={text} />
      <FinalCta text={text} />
    </main>
  )
}
