import fs from 'node:fs'
import path from 'node:path'
import { describe, expect, test } from 'vitest'

const dir = path.resolve(__dirname, '../locales')
const load = (name: string): Record<string, string> =>
  JSON.parse(fs.readFileSync(path.join(dir, name), 'utf8')).translation

const en = load('en.json')
const tr = load('tr.json')
const placeholders = (s: string) => (s.match(/\{\{[^}]*\}\}/g) ?? []).sort().join(',')
const tags = (s: string) => (s.match(/<\/?[a-zA-Z][^>]*>/g) ?? []).sort().join(',')

describe('tr.json', () => {
  test('only contains keys that exist in en.json', () => {
    const unknown = Object.keys(tr).filter((k) => !Object.hasOwn(en, k))
    expect(unknown).toEqual([])
  })

  test('keeps placeholders and markup of the English source', () => {
    const bad = Object.keys(tr).filter(
      (k) => placeholders(en[k]) !== placeholders(tr[k]) || tags(en[k]) !== tags(tr[k])
    )
    expect(bad).toEqual([])
  })

  test('has no empty values and never names an upstream provider', () => {
    const bad = Object.entries(tr).filter(([k, v]) => !v.trim() || /lumia|openrouter/i.test(k + v))
    expect(bad).toEqual([])
  })

  test('covers the customer-facing sign-in and purchase flow', () => {
    for (const key of ['Sign in', 'Sign up', 'Password', 'Email']) {
      if (Object.hasOwn(en, key)) expect(tr[key], key).toBeTruthy()
    }
    expect(Object.keys(tr).length).toBeGreaterThanOrEqual(2600)
  })
})
