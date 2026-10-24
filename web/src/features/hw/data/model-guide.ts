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

/**
 * Plain-language guide for the models HisarWeb AI sells, written for people who
 * are new to AI coding tools. This is the single place for the texts: the
 * catalogue cards use them when the database has no description of its own,
 * and `RECOMMENDED_ORDER` is the default order of the catalogue and the price
 * table.
 */

export type GuideText = {
  tr: string
  en: string
}

export type ModelGuide = {
  /** Short badge shown next to the model name. */
  tag: GuideText
  /** One or two short sentences: what it is good at, with an example. */
  description: GuideText
}

/** Default order of the catalogue ("Recommended"): strongest first. */
export const RECOMMENDED_ORDER = [
  'claude-fable-5-1',
  'claude-opus-5-5',
  'claude-sonnet-5-5',
  'claude-haiku-5-5',
  'claude-sonnet-5',
  'claude-haiku-4-5',
  'gpt-6.1-sol',
  'gpt-6-astra',
  'gpt-6-luna',
  'gemini-3.8-flash',
] as const

export const MODEL_GUIDE: Readonly<Record<string, ModelGuide>> = {
  'claude-fable-5-1': {
    tag: { tr: 'En güçlü', en: 'Most powerful' },
    description: {
      tr: 'En zor işler için en güçlü model. Derin düşünme gerektiren sorunlarda ve saatler süren, çok adımlı görevlerde kullanın; örneğin büyük bir uygulamanın tasarımını baştan kurmak.',
      en: 'The strongest model, for the hardest jobs. Use it for problems that need deep thinking and for multi-step tasks that run for hours, such as designing a large app from scratch.',
    },
  },
  'claude-opus-5-5': {
    tag: { tr: 'Büyük projeler', en: 'Big projects' },
    description: {
      tr: 'Büyük ve karmaşık kodlama işleri ile uzun süren görevler için. Birçok dosyayı kapsayan bir değişikliği baştan sona kendisi yapabilir; Fable’dan daha hızlı ve ucuz.',
      en: 'For large, complex coding work and long-running tasks. It can carry out a change that spans many files from start to finish, faster and cheaper than Fable.',
    },
  },
  'claude-sonnet-5-5': {
    tag: {
      tr: 'Önerilen: en iyi fiyat/performans',
      en: 'Recommended: best value',
    },
    description: {
      tr: 'Her gün kodlama yapmak ve uygulama geliştirmek için en dengeli seçim. Bir açılış sayfasını ya da basit bir uygulamayı hızlıca kurmak için idealdir.',
      en: 'The best balance of price and quality for everyday coding and building apps. Ideal for quickly putting together a landing page or a simple app.',
    },
  },
  'claude-haiku-5-5': {
    tag: {
      tr: 'Önerilen: en hızlı ve ucuz',
      en: 'Recommended: fastest and cheapest',
    },
    description: {
      tr: 'Çok hızlı ve çok ucuz. Sohbet botu, metin özetleme ya da yüzlerce küçük işi (sınıflandırma, bilgi çıkarma) arka arkaya yapmak için uygundur.',
      en: 'Very fast and very cheap. A good fit for a chatbot, summarising text, or running hundreds of small jobs (sorting, extracting facts) in a row.',
    },
  },
  'claude-sonnet-5': {
    tag: { tr: 'Sonnet 5.5 ile çalışır', en: 'Runs on Sonnet 5.5' },
    description: {
      tr: 'Eski ad, mevcut kurulumlar bozulmasın diye duruyor. Bu adla gelen her istek otomatik olarak Sonnet 5.5 ile yanıtlanır, aynı fiyata. Yeni kurulumlarda claude-sonnet-5-5 yazın.',
      en: 'An older name, kept so existing setups keep working. Every request under this name is answered by Sonnet 5.5 automatically, at the same price. Write claude-sonnet-5-5 in new setups.',
    },
  },
  'claude-haiku-4-5': {
    tag: { tr: 'Haiku 5.5 ile çalışır', en: 'Runs on Haiku 5.5' },
    description: {
      tr: 'Eski ad, mevcut kurulumlar bozulmasın diye duruyor. Bu adla gelen her istek otomatik olarak Haiku 5.5 ile yanıtlanır ve Haiku 5.5 fiyatıyla ücretlendirilir. Yeni kurulumlarda claude-haiku-5-5 yazın.',
      en: 'An older name, kept so existing setups keep working. Every request under this name is answered by Haiku 5.5 automatically and billed at the Haiku 5.5 price. Write claude-haiku-5-5 in new setups.',
    },
  },
  'gpt-6.1-sol': {
    tag: { tr: 'Astra’ya yakın, ucuz', en: 'Near Astra, cheaper' },
    description: {
      tr: 'Zor işlerde Astra’ya yakın sonuç verir, ama daha ucuzdur. Günlük kodlama, yazı yazma ve araştırma için iyi bir OpenAI seçeneği.',
      en: 'Gets close to Astra on hard work, but costs less. A good OpenAI choice for everyday coding, writing and research.',
    },
  },
  'gpt-6-astra': {
    tag: { tr: 'OpenAI’nin en güçlüsü', en: 'OpenAI’s strongest' },
    description: {
      tr: 'OpenAI’nin en güçlü modeli, en zorlu işler için. Karmaşık mantık sorunlarında ve büyük kod tabanlarında kullanın.',
      en: 'OpenAI’s strongest model, for the most demanding work. Use it for complex reasoning problems and large codebases.',
    },
  },
  'gpt-6-luna': {
    tag: {
      tr: 'Önerilen: en hızlı ve ucuz',
      en: 'Recommended: fastest and cheapest',
    },
    description: {
      tr: 'Çok hızlı ve çok ucuz. Sohbet botu, özetleme ve çok sayıda küçük, tekrarlayan iş için.',
      en: 'Very fast and very cheap. For a chatbot, summaries and lots of small, repetitive jobs.',
    },
  },
  'gemini-3.8-flash': {
    tag: {
      tr: 'Hızlı, görsel ve uzun belge',
      en: 'Fast, images and long documents',
    },
    description: {
      tr: 'Google’ın hızlı ve ucuz modeli. Görselleri okuyabilir ve çok uzun belgeleri tek seferde işleyebilir, örneğin bir ekran görüntüsünü ya da uzun bir PDF’i.',
      en: 'Google’s fast and cheap model. It can read images and work through very long documents in one go, such as a screenshot or a long PDF.',
    },
  },
}

/** The guide entry of a model, or undefined for a model that is not in it. */
export function modelGuide(
  modelName: string | undefined
): ModelGuide | undefined {
  if (!modelName) return undefined
  return MODEL_GUIDE[modelName]
}

/** Pick the Turkish or English version of a guide text. */
export function guideText(text: GuideText, language: string): string {
  return language.toLowerCase().startsWith('tr') ? text.tr : text.en
}

/**
 * Position of a model in the recommended order; models that are not in the
 * list come after it.
 */
export function recommendedRank(modelName: string | undefined): number {
  const index = (RECOMMENDED_ORDER as readonly string[]).indexOf(
    modelName ?? ''
  )
  return index === -1 ? RECOMMENDED_ORDER.length : index
}
