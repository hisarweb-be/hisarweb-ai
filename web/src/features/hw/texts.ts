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
  navBuy: 'Satın al',
  navCompare: 'Fiyat karşılaştırma',
  navQuality: 'Kalite ölçümleri',
  navStatus: 'Durum',

  landingBadge: 'Türkiye için: TL fiyatlar, KDV dahil',
  landingTitle:
    'Claude, GPT ve Gemini API’si resmî fiyatın yarısına, TL ile ödeyin',
  landingLead:
    'HisarWeb AI, yapay zekâ uygulamanız için tek bir API anahtarı sunar. Her model resmî liste fiyatının %50 altında ve diğer API aracılarından ucuz. Aylık abonelik yok; yalnızca kullandığınız kadar ödersiniz.',
  landingCtaPrimary: 'Ücretsiz hesap aç',
  landingCtaSecondary: 'Fiyatları karşılaştır',
  landingCatalogNote:
    'Claude, GPT ve Gemini ailelerinden seçilmiş modeller tek katalogda; her modelin güncel fiyatı karşılaştırma sayfasında yazar.',

  benefitTitle: 'Neden HisarWeb AI?',
  benefitPaymentTitle: 'TL ile ödeyin',
  benefitPaymentBody:
    'Kredi kartı veya Revolut ile Türk lirası üzerinden ödeme yapın. Gösterilen tüm fiyatlara KDV dahildir.',
  benefitDiscountTitle: 'Her zaman resmî fiyatın yarısı',
  benefitDiscountBody:
    'Tüm modellerde token fiyatı resmî liste fiyatının %50 altındadır, her istekte ve anında. İlk alışverişinizde 1000 TL değerine kadar ayrıca %50 hoş geldin indirimi alırsınız. Aylık abonelik ücreti yok.',
  benefitExpiryTitle: 'Bakiyeniz 30 gün geçerli',
  benefitExpiryBody:
    'Yüklediğiniz bakiye yükleme tarihinden itibaren 30 gün geçerlidir. Kullanmadığınız süre için ödeme yapmazsınız.',

  trialTitle: 'Önce ücretsiz deneyin',
  trialBody:
    'Hesap açtığınızda 250 TL deneme bakiyesi tanımlanır ve 14 gün geçerlidir. Kredi kartı gerekmez.',
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
  qualityTeaserLink: 'Kalite ölçümlerini görün',

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
    '250 TL deneme bakiyesiyle modelleri deneyin; memnun kalırsanız TL ile kredi yükleyin.',
  finalCtaButton: 'Ücretsiz hesap aç',

  topUpTitle: 'Kredi yükle',
  topUpPageTitle: 'Satın al',
  topUpLead:
    'Yüklemek istediğiniz bakiye değerini seçin. Hesabınıza tam değer geçer; ödenecek tutar indirimden sonra hesaplanır.',
  topUpPresetLabel: 'Hazır tutarlar',
  topUpCustomLabel: 'Kendi tutarınız (TL)',
  topUpCustomPlaceholder: 'Örn. 1500',
  topUpValueLabel: 'Bakiye değeri',
  topUpPayableLabel: 'Ödenecek tutar',
  topUpPayableNote: 'İndirimli ödenecek tutarı ödeme sayfasında görürsünüz.',
  topUpDiscountFirst: 'Hoş geldin indirimi (%50, 1000 TL değerine kadar)',
  topUpDiscountRepeat:
    'Sonraki alışverişler: token fiyatları zaten resmî fiyatın yarısı',
  topUpDiscountRule:
    'Token fiyatları her zaman resmî liste fiyatının %50 altındadır. İlk alışverişinizde ayrıca 1000 TL değerine kadar %50 hoş geldin indirimi uygulanır; üzerindeki kısım ve sonraki alışverişler bu indirimli token fiyatıyla kullanılır. İndirim ödeme sırasında otomatik hesaplanır.',
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
  topUpReturnPending: 'Bakiye, ödeme onaydan sonra görünür.',
  topUpReturnBody:
    'Ödeme onaylandığında bakiye kısa süre içinde hesabınıza eklenir. Bakiye hemen görünmezse birkaç dakika sonra yenileyin.',
  topUpRefresh: 'Bakiyeyi yenile',

  topUpPackagesTitle: 'Paketleriniz',
  topUpPackagesLead:
    'Kalan bakiye ve son kullanma tarihi, paket başına gösterilir.',
  topUpPackagesLoading: 'Paketler yükleniyor…',
  topUpPackagesError: 'Paketler okunamadı. Lütfen tekrar deneyin.',

  balanceTitle: 'Bakiyeniz',
  balanceTotal: 'Toplam: {amount}',
  balanceEmpty: 'Henüz bakiyeniz yok. Yukarıdan kredi yükleyebilirsiniz.',
  balancePackageLine: '{source}: {amount} · son kullanma {date}',
  balancePackageQuota: '{quota} kredi',
  balanceNoRate:
    'Bakiyenin TL değeri şu anda hesaplanamıyor: günlük kur henüz yayınlanmadı.',
  packageSourcePurchase: 'Satın alınan bakiye',
  packageSourceFree: 'Ücretsiz deneme',
  packageSourceAffiliate: 'Davet ödülü',
  packageSourceAdmin: 'Yönetici tanımı',

  compareTitle: 'Fiyat karşılaştırma',
  compareLead:
    'Aylık token kullanımınızı girin; resmî liste fiyatı ile HisarWeb AI fiyatını yan yana görün. HisarWeb AI her zaman resmî fiyatın yarısıdır.',
  compareModelLabel: 'Model',
  compareInputLabel: 'Aylık girdi token sayısı',
  compareOutputLabel: 'Aylık çıktı token sayısı',
  compareSubmit: 'Karşılaştır',
  compareResultTitle: 'Karşılaştırma sonucu',
  compareResultLead: '{model} · ayda {input} girdi / {output} çıktı token',
  compareOfficial: 'Resmî liste fiyatı',
  compareOfficialUsd: 'Resmî liste fiyatı (USD)',
  compareOfficialTry: 'Resmî liste fiyatı (TL)',
  compareOursFirst:
    'HisarWeb AI fiyatı (ilk alışveriş, hoş geldin indirimiyle)',
  compareOursRepeat: 'HisarWeb AI fiyatı (her alışverişte)',
  compareSavingFirst: 'İlk alışverişte tasarruf: {amount}',
  compareSavingRepeat: 'Her alışverişte tasarruf: {amount}',
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

  statusTitle: 'Hizmet durumu',
  statusLead:
    'HisarWeb AI modellerinin güncel durumu, son 30 günün kullanılabilirliği ve yaşanan kesintiler.',
  statusOverallOperational: 'Tüm sistemler çalışıyor',
  statusOverallOperationalBody: 'Ölçülen tüm modeller istekleri yanıtlıyor.',
  statusOverallDegraded: 'Hizmette yavaşlama var',
  statusOverallDegradedBody:
    'Bazı istekler yavaş yanıt veriyor; hizmet çalışmaya devam ediyor.',
  statusOverallOutage: 'Kesinti var',
  statusOverallOutageBody:
    'En az bir modelde kesinti var; ekip üzerinde çalışıyor.',
  statusOperational: 'Çalışıyor',
  statusDegraded: 'Yavaşlama',
  statusOutage: 'Kesinti',
  statusUpdatedAt: 'Son güncelleme: {time}',
  statusAutoRefresh: 'Bu sayfa 60 saniyede bir kendini yeniler.',
  statusModelsTitle: 'Modeller',
  statusModelColumn: 'Model',
  statusColumn: 'Durum',
  statusAvailabilityColumn: 'Son {days} gün kullanılabilirlik',
  statusAvailabilityValue: '%{percent}',
  statusNoMeasurement: 'Henüz ölçüm yok',
  statusIncidentsTitle: 'Kesintiler',
  statusOngoingIncident: 'Süren kesinti',
  statusRecentIncidents: 'Son kesintiler',
  statusNoIncidents: 'Bu dönemde kayıtlı kesinti yok.',
  statusIncidentStart: 'Başlangıç: {time}',
  statusIncidentEnd: 'Bitiş: {time}',
  statusIncidentOngoing: 'Devam ediyor',
  statusIncidentDuration: 'Süre: {duration}',
  statusDurationMinutes: '{minutes} dakika',
  statusDurationHours: '{hours} saat {minutes} dakika',
  statusDurationHoursOnly: '{hours} saat',
  statusDurationBelowMinute: '1 dakikadan kısa',
  statusExtensionTitle: 'Kesinti uzarsa bakiyeniz de uzar',
  statusExtensionBody:
    '15 dakikadan uzun süren bir kesintide, ücretli paketlerinizin süresi kesintinin süresi kadar otomatik olarak uzatılır. Bunun için bir şey yapmanız gerekmez.',
  statusLoading: 'Durum bilgisi yükleniyor…',
  statusError: 'Durum bilgisi okunamadı. Lütfen tekrar deneyin.',
  statusEmpty: 'Henüz durum verisi yok',
  statusEmptyBody:
    'Durum ölçümü henüz başlamadı. İlk ölçümden sonra model durumları burada görünecek.',

  bonusEyebrow: 'Başlangıç bonusu',
  bonusTitle: '250 TL başlangıç bonusunuzu alın',
  bonusLead: 'Bonusu almak için 3 kısa adım var. Şu an {step}. adımdasınız.',
  bonusClosed:
    'Telefon doğrulaması şu anda açık değil, bu yüzden bonus henüz tanımlanamıyor. Doğrulama açıldığında bonusu buradan alabileceksiniz; beklemek istemezseniz TL ile bakiye yükleyebilirsiniz.',
  bonusStepEmail: 'E-posta adresinizi ekleyin ve onaylayın',
  bonusStepTelegram: 'Telegram hesabınızı bağlayın',
  bonusStepPhone: 'Telefon numaranızı Telegram botuyla paylaşın',
  bonusButtonEmail: 'E-postayı ekle',
  bonusButtonTelegram: 'Telegram’ı bağla',
  bonusButtonPhone: 'Telegram botunu aç',
  noCreditTitle: 'Bakiyeniz bitti',
  noCreditBody:
    'Bu isteği göndermek için hesabınızda yeterli bakiye yok. Yeni hesaplar 250 TL başlangıç bonusu alır; bonusu henüz almadıysanız aşağıdan talep edin ya da TL ile bakiye yükleyin.',
  noCreditBonusButton: '250 TL başlangıç bonusunu al',
  noCreditBuyButton: 'Bakiye satın al',
  docsTitle: 'API belgeleri',
  docsLead:
    'HisarWeb AI, OpenAI ve Claude ile uyumlu tek bir API sunar. Anahtarınızı bir kez oluşturup uygulamanızda kullanın.',
  docsBaseUrlTitle: 'Temel adres',
  docsBaseUrlBody: 'Tüm istekler aşağıdaki temel adresle başlar:',
  docsKeyTitle: 'API anahtarı oluşturma',
  docsKeyBody:
    'Panoda API anahtarları bölümünü açın, yeni bir anahtar oluşturun ve anahtarı yalnızca kendi sunucunuzda saklayın.',
  docsChatTitle: 'Sohbet tamamlama isteği',
  docsMessagesTitle: 'Claude mesaj isteği',
  docsStreamTitle: 'Akış (stream)',
  docsStreamBody:
    'Yanıtı parça parça almak için isteğe "stream": true ekleyin. Yanıt sunucudan olay akışı (SSE) olarak gelir.',
  docsModelsTitle: 'Hangi modeller ücretsiz?',
  docsModelsBody:
    'Ücretsiz deneme hesabı haiku sınıfı hızlı modelleri kullanabilir; bunlar küçük ve hızlı işler için yeterlidir. Ücretli hesap katalogdaki tüm modelleri kullanabilir.',
  docsModelsFree: 'Ücretsiz: haiku sınıfı modeller (örneğin claude-haiku-4-5).',
  docsModelsPaid:
    'Ücretli: katalogdaki diğer tüm modeller; güncel fiyatlar fiyat karşılaştırma sayfasında yazar.',
  docsErrorsTitle: 'Hata kodları',
  docsError401:
    '401: anahtar geçersiz veya eksik. Anahtarın doğru kopyalandığını kontrol edin.',
  docsError402:
    '402 yetersiz bakiye: hesap bakiyesi bu istek için yetmiyor. Bakiye yükleyip tekrar deneyin.',
  docsError429:
    '429: çok fazla istek gönderildi. Kısa bir süre bekleyip tekrar deneyin.',
  docsLinksTitle: 'Daha fazla',
  docsLinkStatus: 'Servis durumu (7/24)',
  docsLinkCompare: 'Fiyat karşılaştırma',

  aboutDefaultTitle: 'HisarWeb AI hakkında',
  aboutDefaultIntro:
    'HisarWeb AI, yapay zekâ uygulamanız için tek bir API anahtarı sunar. Bakiyeyi Türk lirası ile yüklersiniz ve yalnızca kullandığınız kadar ödersiniz; gösterilen tüm fiyatlara KDV dahildir.',
  aboutDefaultPayment:
    'Ödemeyi kartla veya Revolut ile yapabilirsiniz. Faturalarınızı ve bakiye hareketlerinizi panodan görürsünüz.',
  aboutDefaultStatus:
    'Hizmet durumunu 7/24 durum sayfasından izleyebilirsiniz.',
  aboutDefaultStatusLink: 'Durum sayfasına git',

  retry: 'Tekrar dene',
  rateLimited:
    'Çok fazla istek gönderildi. Lütfen birkaç saniye sonra tekrar deneyin.',
  statusScrollHint: 'Tüm sütunlar için tabloyu yana kaydırın.',
  explainTitle: 'Kredinizle ne alırsınız?',
  explainLead:
    'Yapay zekâ kullanımı token ile ölçülür. Aşağıdaki üç kısa not ve hesaplayıcı, bütçenizin ne kadar yeteceğini anlamanıza yardım eder.',
  explainTokenTitle: 'Token nedir?',
  explainTokenBody:
    'Yapay zekâ metni token denilen küçük parçalar halinde okur. Bir token yaklaşık ¾ kelimedir: 1 milyon token yaklaşık 750.000 kelimeye denk gelir.',
  explainIoTitle: 'Girdi ve çıktı',
  explainIoBody:
    'Girdi, modele gönderdiğiniz her şeydir (sorunuz, kodunuz, dosyalar). Çıktı, modelin yazdığı yanıttır. Çıktının fiyatı girdiden yüksektir.',
  explainCodingTitle: 'Kodlama araçları neden çok girdi harcar?',
  explainCodingBody:
    'Cursor, Cline veya Claude Code gibi araçlar her adımda projenizdeki dosyaları ve önceki konuşmayı modele yeniden gönderir. Harcamanın büyük kısmı kısa komutunuzdan değil, bu gönderilen metinden gelir.',
  calcTitle: 'Bütçem ne kadar yeter?',
  calcBudgetLabel: 'Bütçe',
  calcModelLabel: 'Model',
  calcHoursResult: 'Yaklaşık {low} ile {high} saat aktif kodlama',
  calcProjectsResult: 'Yaklaşık {low} ile {high} küçük proje',
  calcUnavailable: 'Bu model için şu an hesap yapılamıyor.',
  calcAssumption:
    'Yaklaşık, varsayım: Anthropic’in yayımladığı rakamlara göre bir geliştirici ortalama günde yaklaşık {typical} dolar harcar ve kullanıcıların %90’ı günde {heavy} doların altında kalır (resmî liste fiyatıyla). Hesap, günde {hours} saat aktif kodlama, {reference} modelinin liste fiyatı ve küçük bir proje için {projectHours} saat varsayar; HisarWeb AI fiyatına göre yeniden hesaplanır. Gerçek harcama işinize göre değişir.',
  calcSourceLabel: 'Kaynak: Anthropic, maliyet rehberi',
} as const

export type HwTextKey = keyof typeof tr

const en: Record<HwTextKey, string> = {
  navTopUp: 'Top up',
  navBuy: 'Buy credit',
  navCompare: 'Price comparison',
  navQuality: 'Quality results',
  navStatus: 'Status',

  landingBadge: 'For Turkey: prices in TRY, VAT included',
  landingTitle:
    'Claude, GPT and Gemini APIs at half the official price, paid in TRY',
  landingLead:
    'HisarWeb AI gives your application one API key. Every model is 50% below the official list price and cheaper than other API routers. No monthly subscription; you only pay for what you use.',
  landingCtaPrimary: 'Create a free account',
  landingCtaSecondary: 'Compare prices',
  landingCatalogNote:
    'Selected models from the Claude, GPT and Gemini families in one catalog; every current price is on the comparison page.',

  benefitTitle: 'Why HisarWeb AI?',
  benefitPaymentTitle: 'Pay in TRY',
  benefitPaymentBody:
    'Pay in Turkish lira with a card or Revolut. Every price shown includes VAT.',
  benefitDiscountTitle: 'Always half the official price',
  benefitDiscountBody:
    'Every model costs 50% less per token than the official list price, on every request and in real time. Your first purchase also gets a 50% welcome discount on up to TRY 1,000 of value. No monthly subscription.',
  benefitExpiryTitle: 'Your balance lasts 30 days',
  benefitExpiryBody:
    'A top-up stays valid for 30 days from the day you buy it. You pay nothing for time you do not use.',

  trialTitle: 'Try it for free first',
  trialBody:
    'A new account receives TRY 250 of trial credit, valid for 14 days. No card needed.',
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
  qualityTeaserLink: 'See the quality results',

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
    'Try the models with TRY 250 of trial credit; top up in TRY when you are happy.',
  finalCtaButton: 'Create a free account',

  topUpTitle: 'Top up',
  topUpPageTitle: 'Buy credit',
  topUpLead:
    'Choose the balance value you want. Your account receives the full value; the amount you pay follows from the discount.',
  topUpPresetLabel: 'Preset amounts',
  topUpCustomLabel: 'Your own amount (TRY)',
  topUpCustomPlaceholder: 'e.g. 1500',
  topUpValueLabel: 'Balance value',
  topUpPayableLabel: 'You pay',
  topUpPayableNote:
    'The amount you pay after the discount is shown on the payment page.',
  topUpDiscountFirst: 'Welcome discount (50%, up to TRY 1,000 of value)',
  topUpDiscountRepeat:
    'Later purchases: token prices are already half the official price',
  topUpDiscountRule:
    'Token prices are always 50% below the official list price. Your first purchase also gets a 50% welcome discount on up to TRY 1,000 of value; the part above that and later purchases are used at that reduced token price. The discount is applied automatically at payment.',
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
  topUpReturnPending: 'The balance appears after the payment is confirmed.',
  topUpReturnBody:
    'Once the payment is confirmed the balance is added to your account within moments. If it is not there yet, refresh in a few minutes.',
  topUpRefresh: 'Refresh balance',

  topUpPackagesTitle: 'Your packages',
  topUpPackagesLead:
    'The remaining balance and expiry date are shown per package.',
  topUpPackagesLoading: 'Loading the packages…',
  topUpPackagesError: 'The packages could not be loaded. Please try again.',

  balanceTitle: 'Your balance',
  balanceTotal: 'Total: {amount}',
  balanceEmpty: 'You have no balance yet. Top up above.',
  balancePackageLine: '{source}: {amount} · expires {date}',
  balancePackageQuota: '{quota} credits',
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

  statusTitle: 'Service status',
  statusLead:
    'The current state of the HisarWeb AI models, the availability over the last 30 days and the outages that happened.',
  statusOverallOperational: 'All systems are operational',
  statusOverallOperationalBody: 'Every measured model answers requests.',
  statusOverallDegraded: 'The service is slow',
  statusOverallDegradedBody:
    'Some requests are answering slowly; the service keeps working.',
  statusOverallOutage: 'There is an outage',
  statusOverallOutageBody:
    'At least one model is in an outage; the team is working on it.',
  statusOperational: 'Operational',
  statusDegraded: 'Degraded',
  statusOutage: 'Outage',
  statusUpdatedAt: 'Last updated: {time}',
  statusAutoRefresh: 'This page refreshes itself every 60 seconds.',
  statusModelsTitle: 'Models',
  statusModelColumn: 'Model',
  statusColumn: 'Status',
  statusAvailabilityColumn: 'Availability over the last {days} days',
  statusAvailabilityValue: '{percent}%',
  statusNoMeasurement: 'No measurement yet',
  statusIncidentsTitle: 'Outages',
  statusOngoingIncident: 'Ongoing outage',
  statusRecentIncidents: 'Recent outages',
  statusNoIncidents: 'No outage has been recorded in this period.',
  statusIncidentStart: 'Started: {time}',
  statusIncidentEnd: 'Ended: {time}',
  statusIncidentOngoing: 'Still running',
  statusIncidentDuration: 'Duration: {duration}',
  statusDurationMinutes: '{minutes} min',
  statusDurationHours: '{hours} h {minutes} min',
  statusDurationHoursOnly: '{hours} h',
  statusDurationBelowMinute: 'less than a minute',
  statusExtensionTitle: 'A long outage extends your balance too',
  statusExtensionBody:
    'When an outage lasts longer than 15 minutes, the paid packages you hold are extended automatically by the length of the outage. You do not have to do anything for that.',
  statusLoading: 'Loading the service status…',
  statusError: 'The service status could not be loaded. Please try again.',
  statusEmpty: 'No status data yet',
  statusEmptyBody:
    'The status measurement has not started yet. The model states appear here after the first measurement.',

  bonusEyebrow: 'Start bonus',
  bonusTitle: 'Claim your TRY 250 start bonus',
  bonusLead:
    'There are 3 short steps to receive the bonus. You are on step {step}.',
  bonusClosed:
    'Phone verification is not open right now, so the bonus cannot be granted yet. You will be able to claim it here once verification opens; if you do not want to wait, top up in TRY.',
  bonusStepEmail: 'Add and confirm your e-mail address',
  bonusStepTelegram: 'Link your Telegram account',
  bonusStepPhone: 'Share your phone number with the Telegram bot',
  bonusButtonEmail: 'Add e-mail',
  bonusButtonTelegram: 'Link Telegram',
  bonusButtonPhone: 'Open the Telegram bot',
  noCreditTitle: 'You are out of credit',
  noCreditBody:
    'Your account does not have enough credit for this request. New accounts get a TRY 250 start bonus; claim it below if you have not yet, or top up in TRY.',
  noCreditBonusButton: 'Claim the TRY 250 start bonus',
  noCreditBuyButton: 'Buy credit',
  docsTitle: 'API documentation',
  docsLead:
    'HisarWeb AI offers one API that is compatible with OpenAI and Claude. Create your key once and use it in your application.',
  docsBaseUrlTitle: 'Base URL',
  docsBaseUrlBody: 'Every request starts from this base URL:',
  docsKeyTitle: 'Creating an API key',
  docsKeyBody:
    'In the console, open the API keys section, create a new key and keep it on your own server only.',
  docsChatTitle: 'Chat completion request',
  docsMessagesTitle: 'Claude messages request',
  docsStreamTitle: 'Streaming',
  docsStreamBody:
    'Add "stream": true to the request to receive the answer in parts. The answer arrives from the server as a stream of events (SSE).',
  docsModelsTitle: 'Which models are free?',
  docsModelsBody:
    'A free trial account can use the fast haiku-class models; they are enough for small, quick jobs. A paid account can use every model in the catalog.',
  docsModelsFree: 'Free: haiku-class models (for example claude-haiku-4-5).',
  docsModelsPaid:
    'Paid: every other model in the catalog; the current prices are on the price comparison page.',
  docsErrorsTitle: 'Error codes',
  docsError401:
    '401: the key is invalid or missing. Check that the key was copied completely.',
  docsError402:
    '402 insufficient balance: the account balance does not cover this request. Top up and try again.',
  docsError429: '429: too many requests. Wait a moment and try again.',
  docsLinksTitle: 'More',
  docsLinkStatus: 'Service status (24/7)',
  docsLinkCompare: 'Price comparison',

  aboutDefaultTitle: 'About HisarWeb AI',
  aboutDefaultIntro:
    'HisarWeb AI gives your AI application one API key. You top up the balance in Turkish lira and pay only for what you use; every price shown includes VAT.',
  aboutDefaultPayment:
    'You can pay by card or with Revolut. Your invoices and balance movements are in the console.',
  aboutDefaultStatus:
    'You can follow the service status around the clock on the status page.',
  aboutDefaultStatusLink: 'Go to the status page',

  retry: 'Try again',
  rateLimited: 'Too many requests. Please try again in a few seconds.',
  statusScrollHint: 'Scroll the table sideways to see every column.',
  explainTitle: 'What does your credit buy?',
  explainLead:
    'AI usage is measured in tokens. The three short notes and the calculator below help you see how far your budget goes.',
  explainTokenTitle: 'What is a token?',
  explainTokenBody:
    'An AI reads text in small pieces called tokens. One token is about ¾ of a word: 1 million tokens is roughly 750,000 words.',
  explainIoTitle: 'Input and output',
  explainIoBody:
    'Input is everything you send to the model (your question, your code, files). Output is the answer the model writes. Output costs more than input.',
  explainCodingTitle: 'Why coding tools use a lot of input',
  explainCodingBody:
    'Tools such as Cursor, Cline or Claude Code send the files of your project and the earlier conversation to the model again at every step. Most of the spending comes from this sent text, not from your short command.',
  calcTitle: 'How far does my budget go?',
  calcBudgetLabel: 'Budget',
  calcModelLabel: 'Model',
  calcHoursResult: 'About {low} to {high} hours of active coding',
  calcProjectsResult: 'About {low} to {high} small projects',
  calcUnavailable: 'No estimate is available for this model right now.',
  calcAssumption:
    'Approximate, based on assumptions: according to figures published by Anthropic, a developer spends about {typical} dollars per active day on average, and 90% of users stay below {heavy} dollars per day (at official list prices). The estimate assumes {hours} hours of active coding per day, the list price of {reference} and {projectHours} hours for a small project, and is recalculated at the HisarWeb AI price. Real spending depends on your work.',
  calcSourceLabel: 'Source: Anthropic, cost guide',
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

/**
 * The language these pages talk to the server in. `language` in the top-up
 * request only selects the description on the payment page, so it follows the
 * page language and not the customer's locale.
 */
export function useHwLanguage(): HwLanguage {
  const { i18n } = useTranslation()
  return pickHwLanguage(i18n.resolvedLanguage ?? i18n.language)
}
