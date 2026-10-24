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
 * Texts of the "let the AI do the setup" part of `/docs`, for beginners who
 * cannot change the settings themselves. Turkish first, English next to it.
 * The prompts themselves live in `docs-ai-setup-prompts.ts`.
 */
import { fillHwText, useHwLanguage, type HwTextVars } from './texts'

const tr = {
  title: 'Kod bilmiyor musunuz? Kurulumu yapay zekaya yaptırın',
  lead: 'Ayarları kendiniz yapmak zorunda değilsiniz. Aşağıdaki hazır promptu anahtarınızla birlikte Claude Code’a ya da Codex’e yapıştırın. Dosyaları araç kendisi oluşturur, ayarı dener ve sonucu size sade bir dille anlatır.',
  needsLogin:
    'Bu yol, aracı zaten açıp kullanabiliyorsanız çalışır, örneğin Claude ya da ChatGPT aboneliğinizle giriş yaptıysanız. Aracınız henüz hiç çalışmıyorsa 6. adımdaki rehberi izleyin.',

  flowLabel:
    'Akış, dört adım: anahtar oluşturun, aracınızı açın, model ve izin modunu seçin, promptu yapıştırın.',
  flow1: 'Anahtar oluşturun',
  flow2: 'Aracınızı açın',
  flow3: 'Model ve izin modunu seçin',
  flow4: 'Promptu yapıştırın',

  keyTitle: '1. Önce ayrı bir anahtar oluşturun',
  keyBody:
    '3. adımdaki gibi yeni bir API anahtarı oluşturun, adına örneğin kurulum yazın. Anahtar formundaki “Kota ayarları” bölümünde “Sınırsız kota” seçeneğini kapatıp “Kota” alanına küçük bir tutar yazarsanız bu anahtar o tutardan fazlasını harcayamaz. Anahtarı kopyalayın, birazdan prompta yapıştıracaksınız.',
  keyLink: '3. adıma dön: API anahtarı oluşturun',

  toolTitle: '2. Aracınızı açın',
  toolClaude:
    'Claude Code: Claude masaüstü uygulamasında “Code” sekmesini açın ya da terminalde claude yazın.',
  toolCodex: 'Codex: Codex uygulamasını açın ya da terminalde codex yazın.',
  toolVscode: 'VS Code: Claude Code ya da Codex eklentisinin panelini açın.',
  toolFolder:
    'Çalışma klasörü olarak önemli dosyalarınızın olmadığı yeni ve boş bir klasör seçin.',

  modelTitle: '3. Modeli ve izin modunu seçin',
  modelBody:
    'Kurulumu yapan pencerede model pek fark etmez, varsayılan model yeterlidir. Kurulumdan sonra HisarWeb AI ile çalışan pencerede şu modeller ayarlı olur:',
  modelDefault: '{model}: günlük iş için, varsayılan model',
  modelHaiku:
    '{model}: alt ajanlar ve küçük işler için. Ücretsiz deneme anahtarı yalnızca bu modeli çalıştırır.',
  modelOpus: '{model}: yalnızca zor düşünme işleri için',
  modelCodex: '{model}: Codex için',
  modeBody:
    'Kurulum sırasında araç dosya oluşturur ve komut çalıştırır. Her adımda onay sormaması için izin modunu kurulum süresince değiştirebilirsiniz:',
  modeClaudeDesktop:
    'Claude masaüstü uygulaması (Code sekmesi): mod seçicide “Bypass permissions” seçeneğini seçin. Pro ve Max planlarında önce masaüstü ayarlarında “Allow bypass permissions mode” seçeneğini açmanız gerekir.',
  modeVscode:
    'VS Code’daki Claude Code: eklenti ayarlarında “Allow dangerously skip permissions” seçeneğini açın, sonra mod göstergesinden “Bypass permissions” seçeneğini seçin.',
  modeCli:
    'Terminaldeki Claude Code: claude --dangerously-skip-permissions komutuyla başlatın. Bu mod çalışan bir oturumun ortasında açılamaz.',
  modeCodex:
    'Codex (uygulama ve VS Code): mesaj kutusunun altındaki izin seçiminden “Full access” seçeneğini seçin. Terminalde /permissions komutunu kullanın.',
  riskTitle: 'Bu modun riski',
  riskBody:
    'Bu modda araç size sormadan dosya değiştirir ve komut çalıştırır. Yalnızca önemli dosyalarınızın olmadığı ayrı bir klasörde kullanın ve kurulum bitince normal moda geri dönün. İsterseniz normal modda da kurabilirsiniz, araç yalnızca birkaç kez onay ister.',

  promptTitle: '4. Promptu yapıştırın',
  promptBody:
    'Aracınıza uygun promptu kopyalayın, içindeki {placeholder} yazısını kendi anahtarınızla değiştirin ve gönderin. Prompt sohbetin bir parçası olduğu için anahtarınız aracın sağlayıcısına da gider. Bu yüzden kurulum için ayrı ve kotalı bir anahtar öneriyoruz.',
  promptClaudeTitle: 'Claude Code için prompt',
  promptCodexTitle: 'Codex için prompt',
  promptAfter:
    'Araç işi bitirince size bir başlatma komutu verir. HisarWeb AI ile çalışmak istediğinizde o komutla yeni bir pencere açın. Aboneliğinizle çalışan normal pencereniz aynı kalır. Bir şey ters giderse 6. adımdaki rehberin “Sık karşılaşılan sorunlar” bölümüne bakın.',
}

const en: Record<keyof typeof tr, string> = {
  title: 'Do not know how to code? Let the AI do the setup',
  lead: 'You do not have to change the settings yourself. Paste the ready prompt below, together with your key, into Claude Code or Codex. The tool creates the files itself, tests the setup and tells you the result in plain words.',
  needsLogin:
    'This route works when you can already open and use the tool, for example when you are signed in with your Claude or ChatGPT subscription. If your tool does not work at all yet, follow the guide in step 6.',

  flowLabel:
    'Flow in four steps: create a key, open your tool, choose the model and permission mode, paste the prompt.',
  flow1: 'Create a key',
  flow2: 'Open your tool',
  flow3: 'Choose model and permission mode',
  flow4: 'Paste the prompt',

  keyTitle: '1. First create a separate key',
  keyBody:
    'Create a new API key as in step 3 and name it, for example, setup. If you turn off “Unlimited Quota” under “Quota Settings” in the key form and enter a small amount in the “Quota” field, this key cannot spend more than that amount. Copy the key, you will paste it into the prompt in a moment.',
  keyLink: 'Back to step 3: create an API key',

  toolTitle: '2. Open your tool',
  toolClaude:
    'Claude Code: open the “Code” tab in the Claude desktop app, or type claude in a terminal.',
  toolCodex: 'Codex: open the Codex app, or type codex in a terminal.',
  toolVscode: 'VS Code: open the panel of the Claude Code or Codex extension.',
  toolFolder:
    'As the working folder, pick a new and empty folder without important files.',

  modelTitle: '3. Choose the model and the permission mode',
  modelBody:
    'In the window that runs the setup the model hardly matters, the default model is enough. After the setup, the window that works with HisarWeb AI has these models:',
  modelDefault: '{model}: for everyday work, the default model',
  modelHaiku:
    '{model}: for subagents and small jobs. A free trial key only runs this model.',
  modelOpus: '{model}: only for hard thinking work',
  modelCodex: '{model}: for Codex',
  modeBody:
    'During the setup the tool creates files and runs commands. So it does not ask for approval at every step, you can change the permission mode for the time of the setup:',
  modeClaudeDesktop:
    'Claude desktop app (Code tab): choose “Bypass permissions” in the mode selector. On the Pro and Max plans, first turn on “Allow bypass permissions mode” in the desktop settings.',
  modeVscode:
    'Claude Code in VS Code: turn on “Allow dangerously skip permissions” in the extension settings, then choose “Bypass permissions” in the mode indicator.',
  modeCli:
    'Claude Code in a terminal: start it with claude --dangerously-skip-permissions. This mode cannot be turned on in the middle of a running session.',
  modeCodex:
    'Codex (app and VS Code): choose “Full access” in the permission picker below the message box. In a terminal, use the /permissions command.',
  riskTitle: 'The risk of this mode',
  riskBody:
    'In this mode the tool changes files and runs commands without asking you. Use it only in a separate folder without important files, and switch back to the normal mode when the setup is done. You can also set up in the normal mode, the tool then asks for approval a few times.',

  promptTitle: '4. Paste the prompt',
  promptBody:
    'Copy the prompt for your tool, replace the word {placeholder} in it with your own key and send it. The prompt is part of the chat, so your key also goes to the provider of the tool. That is why we recommend a separate key with a quota for the setup.',
  promptClaudeTitle: 'Prompt for Claude Code',
  promptCodexTitle: 'Prompt for Codex',
  promptAfter:
    'When the tool is done it gives you a start command. Whenever you want to work with HisarWeb AI, open a new window with that command. Your normal window that runs on your subscription stays as it is. If something goes wrong, check the “Common problems” part of the guide in step 6.',
}

export type AiSetupTextKey = keyof typeof tr

export const aiSetupTexts = { tr, en } as const

/** Returns one text of the AI setup guide in the active interface language. */
export function useAiSetupText(): (
  key: AiSetupTextKey,
  vars?: HwTextVars
) => string {
  const language = useHwLanguage()
  return (key, vars) => fillHwText(aiSetupTexts[language][key], vars)
}
