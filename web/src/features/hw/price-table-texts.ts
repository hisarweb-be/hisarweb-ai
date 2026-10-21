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

import { fillHwText, pickHwLanguage, type HwTextVars } from './texts'

const tr = {
  title: 'Model fiyat tablosu',
  introWhat:
    'Bu tablo, HisarWeb AI’de kullanabileceğiniz modellerin API fiyatını resmi sağlayıcının ve OpenRouter’ın fiyatıyla yan yana gösterir. Rakamlar belirtilen tarihte kaynaklarından okunmuştur.',
  introToken:
    'Token, yapay zekânın metni işlediği küçük parçadır; İngilizce’de yaklaşık dörtte üç kelime eder, Türkçe metinde kelime başına daha çok token gider. “1M token” bir milyon token demektir. Girdi, modele gönderdiğiniz metindir; çıktı, modelin yazdığı yanıttır ve genellikle daha pahalıdır.',
  introWhy:
    'Neden uygun? Modellerin dolar liste fiyatı resmi fiyatla aynı düzeydedir; HisarWeb AI’de avantaj bakiyeyi TL ile yüklerken gelir: ilk alışverişte 1000 TL’ye kadar %50, sonrasında ve sonraki alışverişlerde %15 indirim. Aşağıdaki tasarruf sütunları yalnızca token liste fiyatını karşılaştırır; bu indirim yüzdelere dahil değildir.',

  tableLabel: 'Model başına 1M token fiyatı: resmi, OpenRouter ve HisarWeb AI',
  colModel: 'Model',
  colOfficial: 'Resmi fiyat',
  colOpenRouter: 'OpenRouter fiyatı',
  colHisarweb: 'HisarWeb AI fiyatı',
  colSaving: 'Tasarruf (HisarWeb AI)',
  colInput: 'Girdi / 1M',
  colOutput: 'Çıktı / 1M',
  colVsOfficial: 'Resmiye göre',
  colVsOpenRouter: 'OpenRouter’a göre',
  missing: '—',
  savingNone: 'Tasarruf yok',
  savingNoneMore: 'Tasarruf yok, {percent} daha pahalı',
  savingNoneZero: '%0',

  notesTitle: 'Notlar',
  noteFx:
    '₺ tutarları 1 USD = {rate} ₺ kuruyla ({date}) hesaplanmıştır ve KDV hariçtir; aşağıdaki hesaplayıcı KDV dahil tutar gösterir.',
  noteSaving:
    'Tasarruf, 1M girdi + 1M çıktı toplam fiyatı üzerinden hesaplanır. “—”, o kaynakta modelin listelenmediği anlamına gelir.',
  noteContext:
    'Resmi fiyatlar standart katman ve kısa bağlam içindir; sağlayıcılar çok uzun bağlamda daha yüksek fiyat uygulayabilir.',
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
    'Why is it cheaper? The dollar list price of the models is at the same level as the official price; the advantage on HisarWeb AI comes when you top up in TRY: 50% off up to 1000 TL on the first purchase, and 15% off beyond that and on later purchases. The saving columns below compare the token list price only; that discount is not in the percentages.',

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

  notesTitle: 'Notes',
  noteFx:
    'Amounts in ₺ use 1 USD = {rate} ₺ ({date}) and exclude KDV (VAT); the calculator below shows amounts including KDV.',
  noteSaving:
    'The saving is calculated on the price of 1M input plus 1M output tokens. “—” means the source does not list the model.',
  noteContext:
    'Official prices are for the standard tier and short context; providers may charge more for very long context.',
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
  const dictionary =
    dictionaries[pickHwLanguage(i18n.resolvedLanguage ?? i18n.language)]
  return (key, vars) => fillHwText(dictionary[key], vars)
}
