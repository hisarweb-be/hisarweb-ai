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
 * HisarWeb AI page texts.
 *
 * The HisarWeb pages (landing, top-up, price comparator, quality meter) carry
 * their own small dictionary instead of keys in `src/i18n/locales`: the full
 * Turkish locale file is written separately, and these pages must not wait for
 * it. The language comes from the interface language i18next already resolved,
 * with Turkish as the default: only an explicit English choice switches a page
 * to English.
 */
import { useTranslation } from 'react-i18next'

export type HwLanguage = 'tr' | 'en'

const tr = {
  navTopUp: 'Kredi yükle',
  navCompare: 'Fiyat karşılaştırma',
  navQuality: 'Kalite ölçümleri',

  landingBadge: 'Türkiye için: TL fiyatlar, KDV dahil',
  landingTitle:
    'Claude ve GPT modellerine uygun fiyatlı API erişimi, TL ile ödeyin',
  landingLead:
    'HisarWeb AI, yapay zekâ uygulamanız için tek bir API anahtarı sunar. Aylık abonelik yok; yalnızca yüklediğiniz bakiyeyi kullanırsınız.',
  landingCtaPrimary: 'Ücretsiz hesap aç',
  landingCtaSecondary: 'Fiyatları karşılaştır',

  benefitTitle: 'Neden HisarWeb AI?',
  benefitPaymentTitle: 'TL ile ödeyin',
  benefitPaymentBody:
    'Kredi kartı veya Revolut ile Türk lirası üzerinden ödeme yapın. Gösterilen tüm fiyatlara KDV dahildir.',
  benefitDiscountTitle: 'İlk alımda %50 indirim',
  benefitDiscountBody:
    'İlk alışverişinizde 1000 TL değerine kadar %50, üzerindeki kısımda ve sonraki alışverişlerde %15 indirim. Aylık abonelik ücreti yok.',
  benefitExpiryTitle: 'Bakiyeniz 30 gün geçerli',
  benefitExpiryBody:
    'Yüklediğiniz bakiye yükleme tarihinden itibaren 30 gün geçerlidir. Kullanmadığınız süre için ödeme yapmazsınız.',

  trialTitle: 'Önce ücretsiz deneyin',
  trialBody:
    'Hesap açtığınızda 100 TL deneme bakiyesi tanımlanır ve 14 gün geçerlidir. Kredi kartı gerekmez.',
  trialCta: 'Deneme hesabı aç',

  stepsTitle: 'Dört adımda başlayın',
  step1Title: 'Kayıt olun',
  step1Body: 'E-posta adresinizle hesap açın.',
  step2Title: 'E-postanızı doğrulayın',
  step2Body: 'Gelen kutunuza gelen bağlantıya dokunun.',
  step3Title: 'Telefonunuzu doğrulayın',
  step3Body: 'Telegram üzerinden tek seferlik doğrulama.',
  step4Title: 'API anahtarı oluşturun',
  step4Body: 'Panodan anahtarınızı oluşturup uygulamanıza ekleyin.',

  compareTeaserTitle: 'Resmî fiyatla yan yana görün',
  compareTeaserBody:
    'Aylık token hacminizi girin; resmî liste fiyatını, HisarWeb AI fiyatını ve tasarrufunuzu görün. Her sonuçta fiyatın kaynağı, fiyat tarihi ve kur tarihi yazar.',
  compareTeaserCta: 'Fiyat karşılaştırmasına git',

  faqTitle: 'Sık sorulan sorular',
  faq1Q: 'API anahtarı nedir?',
  faq1A:
    'Uygulamanızın HisarWeb AI modellerine bağlanmasını sağlayan gizli bir metindir. Anahtarı yalnızca kendi sunucunuzda saklayın.',
  faq2Q: 'Nasıl ödeme yapabilirim?',
  faq2A:
    'Kredi kartı veya Revolut ile TL olarak ödeyebilirsiniz. Ödeme sayfası sipariş sırasında açılır.',
  faq3Q: 'Bakiyem ne kadar süre geçerli?',
  faq3A:
    'Satın aldığınız bakiye 30 gün, ücretsiz deneme bakiyesi 14 gün geçerlidir. Süresi dolan bakiye kullanılamaz.',
  faq4Q: 'Abonelik var mı?',
  faq4A:
    'Hayır. Aylık ücret yok; yalnızca yüklediğiniz bakiyeyi kullanırsınız.',
  faq5Q: 'Hangi modelleri kullanabilirim?',
  faq5A:
    'Modellerin listesi ve güncel fiyatları fiyat karşılaştırma sayfasında yazar.',

  finalCtaTitle: 'Bugün başlayın',
  finalCtaBody:
    '100 TL deneme bakiyesiyle modelleri deneyin; memnun kalırsanız TL ile kredi yükleyin.',
  finalCtaButton: 'Ücretsiz hesap aç',

  topUpTitle: 'Kredi yükle',
  topUpLead:
    'Yüklemek istediğiniz bakiye değerini seçin. Hesabınıza tam değer geçer; ödenecek tutar indirimden sonra hesaplanır.',
  topUpPresetLabel: 'Hazır tutarlar',
  topUpCustomLabel: 'Kendi tutarınız (TL)',
  topUpValueLabel: 'Bakiye değeri',
  topUpPayableLabel: 'Ödenecek tutar',
  topUpDiscountFirst: 'İlk alışveriş indirimi (%50, 1000 TL değerine kadar)',
  topUpDiscountRepeat: 'Sonraki alışveriş indirimi (%15)',
  topUpKdvNote: 'Fiyatlara KDV dahildir.',
  topUpSubmit: 'Ödemeye geç',
  topUpSubmitting: 'Ödeme sayfası açılıyor…',
  topUpRangeNote: 'Tutar {min} ile {max} TL arasında olmalıdır.',
  topUpInvalidAmount: 'Geçerli bir tutar girin.',
  topUpErrorRange: 'Tutar izin verilen aralığın dışında.',
  topUpErrorOpenOrders:
    'Ödenmeyi bekleyen bir siparişiniz var. Önce onu tamamlayın veya süresinin dolmasını bekleyin.',
  topUpErrorUnavailable:
    'Ödeme şu anda kullanılamıyor. Lütfen biraz sonra tekrar deneyin.',
  topUpErrorDefault: 'Sipariş başlatılamadı. Lütfen tekrar deneyin.',
  topUpReturnTitle: 'Ödeme sayfasından döndünüz',
  topUpReturnBody:
    'Ödeme onaylandığında bakiye kısa süre içinde hesabınıza eklenir. Bakiye hemen görünmezse birkaç dakika sonra yenileyin.',
  topUpRefresh: 'Bakiyeyi yenile',

  balanceTitle: 'Bakiyeniz',
  balanceTotal: 'Toplam: {amount}',
  balanceEmpty: 'Henüz bakiyeniz yok. Yukarıdan kredi yükleyebilirsiniz.',
  balancePackageLine: '{source}: {amount} · son kullanma {date}',
  balanceNoRate:
    'Bakiyenin TL değeri şu anda hesaplanamıyor: günlük kur henüz yayınlanmadı.',
  packageSourcePurchase: 'Satın alınan bakiye',
  packageSourceFree: 'Ücretsiz deneme',
  packageSourceAffiliate: 'Davet ödülü',
  packageSourceAdmin: 'Yönetici tanımı',

  compareTitle: 'Fiyat karşılaştırma',
  compareLead:
    'Aylık token kullanımınızı girin; resmî liste fiyatı ile HisarWeb AI fiyatını yan yana görün.',
  compareModelLabel: 'Model',
  compareInputLabel: 'Aylık girdi token sayısı',
  compareOutputLabel: 'Aylık çıktı token sayısı',
  compareSubmit: 'Karşılaştır',
  compareResultTitle: 'Karşılaştırma sonucu',
  compareResultLead: '{model} · ayda {input} girdi / {output} çıktı token',
  compareOfficial: 'Resmî liste fiyatı',
  compareOfficialUsd: 'Resmî liste fiyatı (USD)',
  compareOfficialTry: 'Resmî liste fiyatı (TL)',
  compareOursFirst: 'HisarWeb AI fiyatı (ilk alışveriş)',
  compareOursRepeat: 'HisarWeb AI fiyatı (sonraki alışverişler)',
  compareSavingFirst: 'İlk alışverişte tasarruf: {amount}',
  compareSavingRepeat: 'Sonraki alışverişlerde tasarruf: {amount}',
  compareSource: 'Fiyat kaynağı',
  comparePriceDate: 'Fiyat tarihi',
  compareFxSource: 'Kur kaynağı',
  compareFxDate: 'Kur tarihi',
  compareFxRate: 'Kur (1 USD)',
  compareStaleTitle: 'Fiyat verisi güncel olmayabilir',
  compareStale:
    'Uyarı: fiyat verisi güncel olmayabilir. Son başarılı güncellemenin değerleri gösteriliyor.',
  compareKdv: 'Tüm fiyatlar KDV dahil ve TL olarak gösterilir.',
  compareTokensNote: 'Token sayısı 0 ile {max} arasında olmalıdır.',
  compareErrorDefault: 'Karşılaştırma alınamadı. Lütfen tekrar deneyin.',
  compareErrorModel: 'Bu model satılmıyor veya fiyat verisi yok.',
  compareEmptyTitle: 'Karşılaştırma için bir model ve token sayısı girin.',
  compareLoading: 'Fiyatlar hesaplanıyor…',

  qualityTitle: 'Kalite ölçümleri',
  qualityLead:
    'Modelleri aynı görev setiyle ölçüp ortalama puanı ve yanıt süresini yayınlıyoruz.',
  qualityEmpty: 'İlk ölçüm yakında',
  qualityEmptyBody:
    'Henüz yayınlanmış bir ölçüm yok. İlk ölçüm tamamlandığında sonuçlar burada görünecek.',
  qualityMeasuredAt: 'Ölçüm tarihi',
  qualityTaskCount: 'Görev sayısı',
  qualityModelColumn: 'Model',
  qualityRunLine: '{tasks} görev · görev seti {version}',
  qualityScore: 'Ortalama puan',
  qualityLatency: 'Ortalama yanıt süresi',
  qualityLatencyUnit: '{ms} ms',
  qualitySampleNote:
    'Bu sonuçlar sınırlı sayıda görevden oluşan bir örneklem ölçümüdür; her kullanım senaryosunu temsil etmez.',
  qualityError: 'Ölçüm verisi okunamadı. Lütfen tekrar deneyin.',
  qualityLoading: 'Ölçüm verisi yükleniyor…',
} as const

export type HwTextKey = keyof typeof tr

const en: Record<HwTextKey, string> = {
  navTopUp: 'Top up',
  navCompare: 'Price comparison',
  navQuality: 'Quality results',

  landingBadge: 'For Turkey: prices in TRY, VAT included',
  landingTitle: 'Affordable API access to Claude and GPT models, paid in TRY',
  landingLead:
    'HisarWeb AI gives your application one API key. No monthly subscription; you only spend the balance you top up.',
  landingCtaPrimary: 'Create a free account',
  landingCtaSecondary: 'Compare prices',

  benefitTitle: 'Why HisarWeb AI?',
  benefitPaymentTitle: 'Pay in TRY',
  benefitPaymentBody:
    'Pay in Turkish lira with a card or Revolut. Every price shown includes VAT.',
  benefitDiscountTitle: '50% off your first purchase',
  benefitDiscountBody:
    '50% off up to TRY 1,000 of value on your first purchase, and 15% above that and on later purchases. No monthly subscription.',
  benefitExpiryTitle: 'Your balance lasts 30 days',
  benefitExpiryBody:
    'A top-up stays valid for 30 days from the day you buy it. You pay nothing for time you do not use.',

  trialTitle: 'Try it for free first',
  trialBody:
    'A new account receives TRY 100 of trial credit, valid for 14 days. No card needed.',
  trialCta: 'Create a trial account',

  stepsTitle: 'Start in four steps',
  step1Title: 'Sign up',
  step1Body: 'Create an account with your email address.',
  step2Title: 'Verify your email',
  step2Body: 'Open the link we send to your inbox.',
  step3Title: 'Verify your phone',
  step3Body: 'A one-time check through Telegram.',
  step4Title: 'Create an API key',
  step4Body: 'Create the key in the dashboard and add it to your app.',

  compareTeaserTitle: 'See it next to the official price',
  compareTeaserBody:
    'Enter your monthly token volume and see the official list price, the HisarWeb AI price and your saving. Every result names the source, the price date and the rate date.',
  compareTeaserCta: 'Open the price comparison',

  faqTitle: 'Frequently asked questions',
  faq1Q: 'What is an API key?',
  faq1A:
    'A secret string that lets your application reach the HisarWeb AI models. Keep it on your own server only.',
  faq2Q: 'How can I pay?',
  faq2A:
    'With a card or Revolut, in Turkish lira. The payment page opens while you order.',
  faq3Q: 'How long is my balance valid?',
  faq3A:
    'Balance you buy is valid for 30 days; trial credit for 14 days. Expired balance cannot be used.',
  faq4Q: 'Is there a subscription?',
  faq4A: 'No. There is no monthly fee; you only spend the balance you top up.',
  faq5Q: 'Which models can I use?',
  faq5A:
    'The list of models and their current prices is on the price comparison page.',

  finalCtaTitle: 'Start today',
  finalCtaBody:
    'Try the models with TRY 100 of trial credit; top up in TRY when you are happy.',
  finalCtaButton: 'Create a free account',

  topUpTitle: 'Top up',
  topUpLead:
    'Choose the balance value you want. Your account receives the full value; the amount you pay follows from the discount.',
  topUpPresetLabel: 'Preset amounts',
  topUpCustomLabel: 'Your own amount (TRY)',
  topUpValueLabel: 'Balance value',
  topUpPayableLabel: 'You pay',
  topUpDiscountFirst: 'First-purchase discount (50%, up to TRY 1,000 of value)',
  topUpDiscountRepeat: 'Later-purchase discount (15%)',
  topUpKdvNote: 'Prices include VAT.',
  topUpSubmit: 'Continue to payment',
  topUpSubmitting: 'Opening the payment page…',
  topUpRangeNote: 'The amount must be between {min} and {max} TRY.',
  topUpInvalidAmount: 'Enter a valid amount.',
  topUpErrorRange: 'The amount is outside the allowed range.',
  topUpErrorOpenOrders:
    'You already have an order waiting for payment. Finish it or wait until it expires.',
  topUpErrorUnavailable:
    'Payment is not available right now. Please try again a bit later.',
  topUpErrorDefault: 'The order could not be started. Please try again.',
  topUpReturnTitle: 'You are back from the payment page',
  topUpReturnBody:
    'Once the payment is confirmed the balance is added to your account within moments. If it is not there yet, refresh in a few minutes.',
  topUpRefresh: 'Refresh balance',

  balanceTitle: 'Your balance',
  balanceTotal: 'Total: {amount}',
  balanceEmpty: 'You have no balance yet. Top up above.',
  balancePackageLine: '{source}: {amount} · expires {date}',
  balanceNoRate:
    'The lira value of your balance cannot be shown right now: the daily rate has not been published yet.',
  packageSourcePurchase: 'Purchased balance',
  packageSourceFree: 'Free trial',
  packageSourceAffiliate: 'Referral reward',
  packageSourceAdmin: 'Granted by an administrator',

  compareTitle: 'Price comparison',
  compareLead:
    'Enter your monthly token usage and see the official list price next to the HisarWeb AI price.',
  compareModelLabel: 'Model',
  compareInputLabel: 'Monthly input tokens',
  compareOutputLabel: 'Monthly output tokens',
  compareSubmit: 'Compare',
  compareResultTitle: 'Comparison result',
  compareResultLead: '{model} · {input} input / {output} output tokens a month',
  compareOfficial: 'Official list price',
  compareOfficialUsd: 'Official list price (USD)',
  compareOfficialTry: 'Official list price (TRY)',
  compareOursFirst: 'HisarWeb AI price (first purchase)',
  compareOursRepeat: 'HisarWeb AI price (later purchases)',
  compareSavingFirst: 'Saving on the first purchase: {amount}',
  compareSavingRepeat: 'Saving on later purchases: {amount}',
  compareSource: 'Price source',
  comparePriceDate: 'Price date',
  compareFxSource: 'Rate source',
  compareFxDate: 'Rate date',
  compareFxRate: 'Rate (1 USD)',
  compareStaleTitle: 'The price data may be out of date',
  compareStale:
    'Warning: the price data may be out of date. The values of the last successful update are shown.',
  compareKdv: 'All prices are shown in TRY and include VAT.',
  compareTokensNote: 'The token count must be between 0 and {max}.',
  compareErrorDefault: 'The comparison could not be loaded. Please try again.',
  compareErrorModel: 'This model is not sold here or has no price data.',
  compareEmptyTitle: 'Enter a model and a token count to compare.',
  compareLoading: 'Calculating prices…',

  qualityTitle: 'Quality results',
  qualityLead:
    'We measure the models on the same task set and publish the average score and response time.',
  qualityEmpty: 'First measurement coming soon',
  qualityEmptyBody:
    'No measurement has been published yet. The results appear here once the first one is complete.',
  qualityMeasuredAt: 'Measured on',
  qualityTaskCount: 'Tasks',
  qualityModelColumn: 'Model',
  qualityRunLine: '{tasks} tasks · task set {version}',
  qualityScore: 'Average score',
  qualityLatency: 'Average response time',
  qualityLatencyUnit: '{ms} ms',
  qualitySampleNote:
    'These results are a sample over a limited task set; they do not represent every use case.',
  qualityError: 'The measurement data could not be loaded. Please try again.',
  qualityLoading: 'Loading the measurement…',
}

export const hwTexts: Record<HwLanguage, Record<HwTextKey, string>> = { tr, en }

/**
 * Picks the dictionary for an interface language. Turkish is the default of the
 * site, so anything that is not an explicit English choice reads Turkish.
 */
export function pickHwLanguage(language?: string | null): HwLanguage {
  if (language?.trim().toLowerCase().startsWith('en')) return 'en'
  return 'tr'
}

export type HwTextVars = Record<string, string | number>

/** Replaces `{name}` placeholders; an unknown placeholder is left as it is. */
export function fillHwText(text: string, vars?: HwTextVars): string {
  if (!vars) return text
  return text.replaceAll(/\{(\w+)\}/g, (match, name: string) =>
    name in vars ? String(vars[name]) : match
  )
}

/** Returns the text of the active interface language, Turkish by default. */
export function useHwText(): (key: HwTextKey, vars?: HwTextVars) => string {
  const { i18n } = useTranslation()
  const dictionary =
    hwTexts[pickHwLanguage(i18n.resolvedLanguage ?? i18n.language)]
  return (key, vars) => fillHwText(dictionary[key], vars)
}
