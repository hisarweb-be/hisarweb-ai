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
 * Texts of the model price table on the price comparison page.
 *
 * They live next to, not inside, `texts.ts` so the table does not depend on
 * that dictionary. The language choice is the same (`pickHwLanguage`): Turkish
 * by default, English only when the interface language is English.
 */
import { useTranslation } from 'react-i18next'

import { pickHwLanguage, useHwCurrencyText, type HwTextVars } from './texts'

const tr = {
  title: 'Model fiyat tablosu',
  introWhat:
    'Bu tablo, HisarWeb AI’de kullanabileceğiniz modellerin API fiyatını sağlayıcının kendi fiyatıyla ve OpenRouter’ın fiyatıyla yan yana gösterir. Rakamlar belirtilen tarihte kaynaklarından okunmuştur.',
  introToken:
    'Token, yapay zekânın metni işlediği küçük parçadır; İngilizce’de yaklaşık dörtte üç kelime eder, Türkçe metinde kelime başına daha çok token gider. “1M token” bir milyon token demektir. Girdi, modele gönderdiğiniz metindir; çıktı, modelin yazdığı yanıttır ve genellikle daha pahalıdır.',
  introWhy:
    'Neden HisarWeb AI? Her modelin token fiyatı sağlayıcının liste fiyatının yarısıdır, her istekte ve anında. OpenRouter aynı modelleri sağlayıcının liste fiyatından veya daha pahalıya satar ve kredi alımında ayrıca hizmet bedeli alır. Anthropic’in kendi ön ödemeli kullanım kredileri en fazla %30 indirim verir ve peşin ödeme ister. HisarWeb AI’de taahhüt yok, ödeme {currencyName} ile ve ilk alışverişte {firstCap} değerine kadar ayrıca %50 hoş geldin indirimi var; bu ek indirim aşağıdaki yüzdelere dahil değildir.',

  tableLabel:
    'Model başına 1M token fiyatı: sağlayıcı, OpenRouter ve HisarWeb AI',
  colModel: 'Model',
  colOfficial: 'Sağlayıcı fiyatı',
  colOpenRouter: 'OpenRouter fiyatı',
  colHisarweb: 'HisarWeb AI fiyatı',
  colSaving: 'Tasarruf (HisarWeb AI)',
  colInput: 'Girdi / 1M',
  colOutput: 'Çıktı / 1M',
  colVsOfficial: 'Sağlayıcıya göre',
  colVsOpenRouter: 'OpenRouter’a göre',
  missing: '—',
  savingNone: 'Tasarruf yok',
  savingNoneMore: 'Tasarruf yok, {percent} daha pahalı',
  savingNoneZero: '%0',
  savingPill: '{percent} tasarruf',
  bestPrice: 'En iyi fiyat',

  notesTitle: 'Notlar',
  noteFxForeign:
    '{symbol} tutarları 1 USD = {rate} {symbol} kuruyla hesaplanmıştır.',
  noteFx:
    '₺ tutarları 1 USD = {rate} ₺ kuruyla ({date}) hesaplanmıştır ve KDV hariçtir; aşağıdaki hesaplayıcı KDV dahil tutar gösterir.',
  noteSaving:
    'Tasarruf, 1M girdi + 1M çıktı toplam fiyatı üzerinden hesaplanır. “—”, o kaynakta modelin listelenmediği anlamına gelir.',
  noteContext:
    'Sağlayıcı fiyatları standart katman ve kısa bağlam içindir; sağlayıcılar çok uzun bağlamda daha yüksek fiyat uygulayabilir.',
  noteOthers:
    'Anthropic toplu işlemede (Batch API) de %50 indirim verir, ancak yanıt 24 saate kadar gecikebilir. HisarWeb AI’deki %50 indirim her gerçek zamanlı istekte geçerlidir.',
  sourcesTitle: 'Kaynaklar',
  sourceDate: 'Ölçüm tarihi: {date}',
} as const

export type PriceTableTextKey = keyof typeof tr

const en: Record<PriceTableTextKey, string> = {
  title: 'Model price table',
  introWhat:
    'This table shows the API price of the models you can use on HisarWeb AI next to the official provider price and the OpenRouter price. The figures were read from their sources on the date shown.',
  introToken:
    'A token is the small piece of text an AI model works with; in English it is about three quarters of a word, and Turkish text needs more tokens per word. “1M tokens” means one million tokens. Input is the text you send to the model; output is the answer it writes, which usually costs more.',
  introWhy:
    'Why HisarWeb AI? Every model costs half the official list price per token, on every request and in real time. OpenRouter sells the same models at the official price or more and adds a fee when you buy credits. Anthropic’s own prepaid usage credits give at most 30% off and need payment up front. On HisarWeb AI there is no commitment, you pay in {currencyName}, and your first purchase gets an extra 50% welcome discount on up to {firstCap}; that extra discount is not in the percentages below.',

  tableLabel:
    'Price per 1M tokens per model: official, OpenRouter and HisarWeb AI',
  colModel: 'Model',
  colOfficial: 'Official price',
  colOpenRouter: 'OpenRouter price',
  colHisarweb: 'HisarWeb AI price',
  colSaving: 'Saving (HisarWeb AI)',
  colInput: 'Input / 1M',
  colOutput: 'Output / 1M',
  colVsOfficial: 'vs official',
  colVsOpenRouter: 'vs OpenRouter',
  missing: '—',
  savingNone: 'No saving',
  savingNoneMore: 'No saving, {percent} more expensive',
  savingNoneZero: '0%',
  savingPill: '{percent} saved',
  bestPrice: 'Best price',

  notesTitle: 'Notes',
  noteFxForeign: 'Amounts in {symbol} use 1 USD = {rate} {symbol}.',
  noteFx:
    'Amounts in ₺ use 1 USD = {rate} ₺ ({date}) and exclude KDV (VAT); the calculator below shows amounts including KDV.',
  noteSaving:
    'The saving is calculated on the price of 1M input plus 1M output tokens. “—” means the source does not list the model.',
  noteContext:
    'Official prices are for the standard tier and short context; providers may charge more for very long context.',
  noteOthers:
    'Anthropic also gives 50% off for batch processing (Batch API), but answers can take up to 24 hours. The 50% on HisarWeb AI applies to every real-time request.',
  sourcesTitle: 'Sources',
  sourceDate: 'Measured on: {date}',
}

const dictionaries: Record<'tr' | 'en', Record<PriceTableTextKey, string>> = {
  tr,
  en,
}

/** Returns the price table text of the active interface language. */
export function usePriceTableText(): (
  key: PriceTableTextKey,
  vars?: HwTextVars
) => string {
  const { i18n } = useTranslation()
  const language = pickHwLanguage(i18n.resolvedLanguage ?? i18n.language)
  return useHwCurrencyText(dictionaries[language], language)
}
