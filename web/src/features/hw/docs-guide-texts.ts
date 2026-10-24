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
 * Texts of the beginner guide at the top of `/docs`: what HisarWeb AI is, the
 * steps from sign-up to a first answer, and the title of the program picker.
 *
 * Same language choice as the rest of the HisarWeb pages (`pickHwLanguage`):
 * Turkish by default, English only when the interface language is English.
 */
import { useTranslation } from 'react-i18next'

import { pickHwLanguage, useHwCurrencyText, type HwTextVars } from './texts'

const tr = {
  whatTitle: 'HisarWeb AI nedir?',
  whatBody:
    'HisarWeb AI, Claude ve GPT gibi yapay zekâ modellerini tek bir yerden, {currencyName} ile ve kredi kartı olmadan kullanmanızı sağlar. Bir hesap açarsınız, bir anahtar oluşturursunuz ve modeli kendi araçlarınızda, örneğin Claude Code’da, kullanırsınız. Kod bilmeniz gerekmez.',
  whatFree:
    'Yeni hesaplar {bonus} başlangıç bonusu alır; bonus 14 gün geçerlidir. Aşağıdaki adımlarla ilk yanıtınızı birkaç dakikada görürsünüz.',

  videoTitle: 'Video rehber',
  videoBody:
    'Aynı adımları kısa bir videoda izleyin. Videoda ses yoktur; ekrandaki yazılar Türkçedir.',
  videoFallback: 'Tarayıcınız bu videoyu oynatamıyor.',

  stepsTitle: 'Adım adım: kayıttan ilk yanıta',
  step: 'Adım {number}',

  step1Title: 'Hesap açın',
  step1Body:
    'Sağ üstteki “Deneme hesabı aç” düğmesine basın. Bir kullanıcı adı ve en az 8 karakterlik bir parola seçin, sözleşmeyi onaylayın ve “Hesap oluştur” düğmesine basın.',
  step1Alt: 'Hesap oluşturma formu: kullanıcı adı, parola ve onay kutusu',

  step2Title: '{bonus} başlangıç bonusunu alın',
  step2Body:
    'Giriş yaptığınızda Genel Bakış sayfasının en üstünde “{bonus} başlangıç bonusunuzu alın” kartı görünür ve hangi adımın kaldığını yazar: e-posta adresinizi ekleyin ve onaylayın, Telegram hesabınızı bağlayın, telefon numaranızı HisarWeb botuyla paylaşın. Son adım bitince bakiyeniz {bonus} olarak görünür. Doğrulama henüz açık değilse kart bunu açıkça yazar; o zaman bakiyeyi “Satın al” sayfasından yükleyebilirsiniz.',
  step2Alt: 'Genel Bakış sayfasında başlangıç bonusu kartı ve üç adım',

  step3Title: 'API anahtarı oluşturun',
  step3Body:
    'Soldaki menüden “API Anahtarları” sayfasını açın ve “API Anahtarı Oluştur” düğmesine basın. Ad olarak claude-code yazın. Grup olarak bonus bakiyeniz için “Ücretsiz deneme”yi, satın aldığınız bakiye için “Ücretli”yi seçin ve kaydedin.',
  step3Alt: 'API anahtarı oluşturma paneli: ad ve grup seçimi',
  step3Body2:
    'Anahtar listede görünür. Yanındaki kopyala simgesine basarak anahtarı kopyalayın. Anahtar bir parola gibidir: kimseyle paylaşmayın ve herkese açık yerlere yazmayın.',
  step3Alt2: 'API anahtarı listesi, kopyala simgesiyle',

  step4Title: 'Bakiye yükleyin (isteğe bağlı)',
  step4Body:
    'Başlangıç bonusu bittiğinde soldaki menüden “Satın al” sayfasını açın. Ödeme {currencyName} ile yapılır. Token fiyatları her zaman resmî fiyatın yarısıdır; ilk alışverişte {firstCap} değerine kadar ayrıca %50 hoş geldin indirimi vardır. Yüklediğiniz bakiye 30 gün geçerlidir; aylık abonelik ücreti yoktur.',
  step4Link: 'Satın al sayfasına git',

  step5Title: 'Playground’da deneyin',
  step5Body:
    'Soldaki menüden “Playground” sayfasını açın, bir model seçin, sorunuzu yazıp “Gönder”e basın. Yanıt geldiyse hesabınız ve bakiyeniz çalışıyor demektir. Bakiye yoksa ekranda sizi yönlendiren bir uyarı çıkar.',
  step5Alt: 'Playground sayfası: model seçimi ve mesaj kutusu',

  step6Title: 'Programınızı seçin',

  devTitle: 'Geliştiriciler için',
  devLead:
    'Kendi uygulamanızdan API’yi doğrudan çağırmak isterseniz aşağıdaki bilgiler yeterlidir.',
  codeSamplesTitle: 'Kod örnekleri',
}

const en: typeof tr = {
  whatTitle: 'What is HisarWeb AI?',
  whatBody:
    'HisarWeb AI lets you use AI models such as Claude and GPT from one place, paying in {currencyLong} and without a credit card. You open an account, create a key and use the model in your own tools, for example Claude Code. You do not need to know how to code.',
  whatFree:
    'New accounts receive a {bonus} start bonus, valid for 14 days. The steps below take you to your first answer in a few minutes.',

  videoTitle: 'Video guide',
  videoBody:
    'Watch the same steps in a short video. It has no sound; the on-screen text is in Turkish.',
  videoFallback: 'Your browser cannot play this video.',

  stepsTitle: 'Step by step: from sign-up to your first answer',
  step: 'Step {number}',

  step1Title: 'Create an account',
  step1Body:
    'Press “Open a trial account” at the top right. Choose a username and a password of at least 8 characters, accept the agreement and press “Create account”.',
  step1Alt: 'Sign-up form with username, password and the agreement checkbox',

  step2Title: 'Claim your {bonus} start bonus',
  step2Body:
    'After you sign in, the top of the Overview page shows a “Claim your {bonus} start bonus” card that names the step still open: add and confirm your email address, connect your Telegram account, share your phone number with the HisarWeb bot. When the last step is done your balance shows {bonus}. If verification is not open yet, the card says so plainly; then you can add balance on the “Buy” page.',
  step2Alt: 'Overview page with the start bonus card and its three steps',

  step3Title: 'Create an API key',
  step3Body:
    'Open “API Keys” in the left menu and press “Create API key”. Type claude-code as the name. Choose the “Free trial” group for your bonus balance, or “Paid” for balance you bought, and save.',
  step3Alt: 'Create API key panel with the name and group fields',
  step3Body2:
    'The key appears in the list. Press the copy icon next to it. A key is like a password: do not share it and do not write it in public places.',
  step3Alt2: 'API key list with the copy icon',

  step4Title: 'Add credit (optional)',
  step4Body:
    'When the start bonus is used up, open “Buy” in the left menu. You pay in {currencyLong}. Token prices are always half the official price, and your first purchase gets an extra 50% welcome discount on up to {firstCap}. Credit stays valid for 30 days; there is no monthly fee.',
  step4Link: 'Go to the buy page',

  step5Title: 'Try it in the Playground',
  step5Body:
    'Open “Playground” in the left menu, choose a model, type your question and press “Send”. If an answer comes back, your account and balance work. Without balance, a notice tells you what to do.',
  step5Alt: 'Playground page with the model picker and the message box',

  step6Title: 'Choose your program',

  devTitle: 'For developers',
  devLead:
    'If you want to call the API directly from your own application, the information below is enough.',
  codeSamplesTitle: 'Code samples',
}

export type DocsGuideTextKey = keyof typeof tr

const dictionaries = { tr, en }

/** Returns one guide text of the active interface language. */
export function useDocsGuideText(): (
  key: DocsGuideTextKey,
  vars?: HwTextVars
) => string {
  const { i18n } = useTranslation()
  const language = pickHwLanguage(i18n.resolvedLanguage ?? i18n.language)
  return useHwCurrencyText(dictionaries[language], language)
}
