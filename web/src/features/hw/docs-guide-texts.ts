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
 * steps from sign-up to a first answer, and how to connect Claude Code.
 *
 * Same language choice as the rest of the HisarWeb pages (`pickHwLanguage`):
 * Turkish by default, English only when the interface language is English.
 */
import { useTranslation } from 'react-i18next'

import { fillHwText, pickHwLanguage, type HwTextVars } from './texts'

const tr = {
  whatTitle: 'HisarWeb AI nedir?',
  whatBody:
    'HisarWeb AI, Claude ve GPT gibi yapay zekâ modellerini tek bir yerden, TL ile ve kredi kartı olmadan kullanmanızı sağlar. Bir hesap açarsınız, bir anahtar oluşturursunuz ve modeli kendi araçlarınızda, örneğin Claude Code’da, kullanırsınız. Kod bilmeniz gerekmez.',
  whatFree:
    'Yeni hesaplar 250 TL startbonus alır; bonus 14 gün geçerlidir. Aşağıdaki adımlarla ilk yanıtınızı birkaç dakikada görürsünüz.',

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

  step2Title: '250 TL startbonusunu alın',
  step2Body:
    'Giriş yaptığınızda Genel Bakış sayfasının en üstünde “250 TL startbonusunu al” kartı görünür ve hangi adımın kaldığını yazar: e-posta adresinizi ekleyin ve onaylayın, Telegram hesabınızı bağlayın, telefon numaranızı HisarWeb botuyla paylaşın. Son adım bitince bakiyeniz 250 TL olarak görünür. Doğrulama henüz açık değilse kart bunu açıkça yazar; o zaman bakiyeyi “Satın al” sayfasından yükleyebilirsiniz.',
  step2Alt: 'Genel Bakış sayfasında startbonus kartı ve üç adım',

  step3Title: 'API anahtarı oluşturun',
  step3Body:
    'Soldaki menüden “API Anahtarları” sayfasını açın ve “API Anahtarı Oluştur” düğmesine basın. Ad olarak claude-code yazın. Grup olarak bonus bakiyeniz için “Ücretsiz deneme”yi, satın aldığınız bakiye için “Ücretli”yi seçin ve kaydedin.',
  step3Alt: 'API anahtarı oluşturma paneli: ad ve grup seçimi',
  step3Body2:
    'Anahtar listede görünür. Yanındaki kopyala simgesine basarak anahtarı kopyalayın. Anahtar bir parola gibidir: kimseyle paylaşmayın ve herkese açık yerlere yazmayın.',
  step3Alt2: 'API anahtarı listesi, kopyala simgesiyle',

  step4Title: 'Bakiye yükleyin (isteğe bağlı)',
  step4Body:
    'Startbonus bittiğinde soldaki menüden “Satın al” sayfasını açın. Ödeme TL ile yapılır. İlk alışverişte 1000 TL değerine kadar %50, üzerindeki kısımda ve sonraki alışverişlerde %15 indirim vardır. Yüklediğiniz bakiye 30 gün geçerlidir; aylık abonelik ücreti yoktur.',
  step4Link: 'Satın al sayfasına git',

  step5Title: 'Playground’da deneyin',
  step5Body:
    'Soldaki menüden “Playground” sayfasını açın, bir model seçin, sorunuzu yazıp “Gönder”e basın. Yanıt geldiyse hesabınız ve bakiyeniz çalışıyor demektir. Bakiye yoksa ekranda sizi yönlendiren bir uyarı çıkar.',
  step5Alt: 'Playground sayfası: model seçimi ve mesaj kutusu',

  step6Title: 'Claude Code’u HisarWeb AI’ye bağlayın',
  step6Body:
    'Claude Code, Anthropic’in kodlama yardımcısıdır. İki yoldan birini seçin: komut satırı (CLI) ya da masaüstü uygulaması.',

  cliTitle: 'Komut satırı (CLI)',
  cliInstallTitle: 'a) Claude Code’u kurun',
  cliInstallWin: 'Windows PowerShell:',
  cliInstallUnix: 'macOS, Linux veya WSL:',
  cliSettingsTitle: 'b) Ayar dosyasını yazın',
  cliSettingsBody:
    'Şu dosyayı açın, yoksa oluşturun: macOS ve Linux’ta ~/.claude/settings.json, Windows’ta %USERPROFILE%\\.claude\\settings.json. Aşağıdaki metni yapıştırın ve BURAYA_ANAHTARINIZ yerine 3. adımda kopyaladığınız anahtarı yazın.',
  cliSettingsNote:
    'Adres https://api.hisarweb.be olmalı, sonuna /v1 eklemeyin; Claude Code gerisini kendisi ekler. Ücretsiz deneme anahtarı yalnızca haiku sınıfı modelleri çalıştırır: o durumda model satırlarının hepsine claude-haiku-4-5 yazın.',
  cliRunTitle: 'c) Başlatın',
  cliRunBody:
    'Bir proje klasöründe terminali açıp claude yazın. Anthropic hesabıyla giriş yapmanız gerekmez. Claude Code “model listesinde yok” türünde bir uyarı gösterebilir; bu zararsızdır, model çalışır.',
  cliTested:
    'Bu adımlar 21 Ekim 2026’da gerçek bir anahtarla denendi: Claude Code, api.hisarweb.be üzerinden yanıt aldı.',

  desktopTitle: 'Masaüstü uygulaması',
  desktopBody:
    'Claude’un masaüstü uygulaması kendi ağ geçidinize “üçüncü taraf çıkarım” modu ile bağlanır. Adımlar Claude’un resmî belgesinden alınmıştır:',
  desktopSteps: [
    'Claude masaüstü uygulamasını claude.com/download adresinden kurun ve açın; Anthropic hesabıyla giriş yapmayın.',
    'Yardım, Sorun giderme, Geliştirici modunu etkinleştir yolunu izleyin.',
    'Geliştirici, Üçüncü taraf çıkarımı yapılandır… menüsünü açın.',
    'Sağlayıcı olarak Gateway’i seçin. Gateway base URL alanına https://api.hisarweb.be yazın; çalışmazsa sonuna /v1 ekleyip tekrar deneyin.',
    'Gateway API key alanına anahtarınızı yapıştırın. Credential kind olarak Static API key, kimlik doğrulama için Bearer seçin.',
    'Model listesine claude-sonnet-5-5 ve claude-haiku-4-5 adlarını ekleyin; ilk yazılan varsayılan olur.',
    'Apply Changes’e, sonra Save & Restart’a basın. Uygulama yeniden açılınca üçüncü taraf modunda başlatmayı seçin.',
  ],

  devTitle: 'Geliştiriciler için',
  devLead:
    'Kendi uygulamanızdan API’yi doğrudan çağırmak isterseniz aşağıdaki bilgiler yeterlidir.',
  codeSamplesTitle: 'Kod örnekleri',
}

const en: typeof tr = {
  whatTitle: 'What is HisarWeb AI?',
  whatBody:
    'HisarWeb AI lets you use AI models such as Claude and GPT from one place, paying in Turkish lira and without a credit card. You open an account, create a key and use the model in your own tools, for example Claude Code. You do not need to know how to code.',
  whatFree:
    'New accounts receive a 250 TL start bonus, valid for 14 days. The steps below take you to your first answer in a few minutes.',

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

  step2Title: 'Claim your 250 TL start bonus',
  step2Body:
    'After you sign in, the top of the Overview page shows a “Claim your 250 TL start bonus” card that names the step still open: add and confirm your email address, connect your Telegram account, share your phone number with the HisarWeb bot. When the last step is done your balance shows 250 TL. If verification is not open yet, the card says so plainly; then you can add balance on the “Buy” page.',
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
    'When the start bonus is used up, open “Buy” in the left menu. You pay in Turkish lira. Your first purchase gets 50% off up to 1000 TL, the part above that and later purchases get 15% off. Credit stays valid for 30 days; there is no monthly fee.',
  step4Link: 'Go to the buy page',

  step5Title: 'Try it in the Playground',
  step5Body:
    'Open “Playground” in the left menu, choose a model, type your question and press “Send”. If an answer comes back, your account and balance work. Without balance, a notice tells you what to do.',
  step5Alt: 'Playground page with the model picker and the message box',

  step6Title: 'Connect Claude Code to HisarWeb AI',
  step6Body:
    'Claude Code is Anthropic’s coding assistant. Pick one of two ways: the command line (CLI) or the desktop app.',

  cliTitle: 'Command line (CLI)',
  cliInstallTitle: 'a) Install Claude Code',
  cliInstallWin: 'Windows PowerShell:',
  cliInstallUnix: 'macOS, Linux or WSL:',
  cliSettingsTitle: 'b) Write the settings file',
  cliSettingsBody:
    'Open this file, or create it: ~/.claude/settings.json on macOS and Linux, %USERPROFILE%\\.claude\\settings.json on Windows. Paste the text below and replace BURAYA_ANAHTARINIZ with the key you copied in step 3.',
  cliSettingsNote:
    'The address must be https://api.hisarweb.be without /v1 at the end; Claude Code adds the rest. A free trial key only runs haiku-class models: in that case write claude-haiku-4-5 on every model line.',
  cliRunTitle: 'c) Start it',
  cliRunBody:
    'Open a terminal in a project folder and type claude. You do not need to sign in with an Anthropic account. Claude Code may print a notice that the model is not in its list; that is harmless and the model works.',
  cliTested:
    'These steps were tried on 21 October 2026 with a real key: Claude Code received an answer through api.hisarweb.be.',

  desktopTitle: 'Desktop app',
  desktopBody:
    'The Claude desktop app connects to your own gateway through its “third-party inference” mode. The steps come from Claude’s official documentation:',
  desktopSteps: [
    'Install the Claude desktop app from claude.com/download and open it; do not sign in with an Anthropic account.',
    'Follow Help, Troubleshooting, Enable Developer Mode.',
    'Open Developer, Configure Third-Party Inference…',
    'Choose Gateway as the provider. Type https://api.hisarweb.be in Gateway base URL; if that fails, add /v1 at the end and try again.',
    'Paste your key into Gateway API key. Choose Static API key as the credential kind and Bearer as the authentication.',
    'Add claude-sonnet-5-5 and claude-haiku-4-5 to the model list; the first one becomes the default.',
    'Press Apply Changes, then Save & Restart. When the app reopens, choose to start in third-party mode.',
  ],

  devTitle: 'For developers',
  devLead:
    'If you want to call the API directly from your own application, the information below is enough.',
  codeSamplesTitle: 'Code samples',
}

export type DocsGuideTextKey = Exclude<keyof typeof tr, 'desktopSteps'>

const dictionaries = { tr, en }

function useDictionary() {
  const { i18n } = useTranslation()
  return dictionaries[pickHwLanguage(i18n.resolvedLanguage ?? i18n.language)]
}

/** Returns one guide text of the active interface language. */
export function useDocsGuideText(): (
  key: DocsGuideTextKey,
  vars?: HwTextVars
) => string {
  const dictionary = useDictionary()
  return (key, vars) => fillHwText(dictionary[key], vars)
}

/** Returns the Claude Desktop steps of the active interface language. */
export function useDocsDesktopSteps(): readonly string[] {
  return useDictionary().desktopSteps
}
