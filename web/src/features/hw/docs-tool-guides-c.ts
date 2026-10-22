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
/** Guides: Cursor and Google Antigravity (the two honest "not for free" tabs). */
import {
  block,
  KEY_PLACEHOLDER,
  MODEL_DEFAULT,
  OPENAI_BASE_URL,
  type ToolGuide,
} from './docs-tool-guides-base'

const cursor: ToolGuide = {
  id: 'cursor',
  name: 'Cursor',
  status: 'needs-paid-plan',
  protocol: 'openai',
  testedWith: {
    tr: 'Cursor 3.24, forum ve belgeler, 22 Ekim 2026’da incelendi; kendi makinemizde denenmedi',
    en: 'Cursor 3.24, forum and documentation, researched 22 Oct 2026; not tried on our own machine',
  },
  summary: {
    tr: 'Cursor, kodlama yardımcısı yerleşik bir editördür. Kendi anahtarınızı bağlamak için bir ayar vardır, ama ücretsiz planda ajan için çalışmaz.',
    en: 'Cursor is an editor with a built-in coding assistant. There is a setting for connecting your own key, but on the free plan it does not work for the agent.',
  },
  limit: {
    tr: 'Dürüst cevap: Cursor’un ücretsiz planında ajan ve düzenleme kipleri kendi (özel) modelinizle çalışmaz; bu bilerek böyle tasarlanmıştır. Kendi OpenAI anahtarınızı ve adresinizi yazabileceğiniz ayar yalnızca ücretli bir Cursor planında işe yarar. Bunun için bir çözüm yolu ya da hile yok. Ücretli plan almak istemiyorsanız aşağıdaki ücretsiz programlardan birini kullanın. Ücretli bir planınız varsa aşağıdaki adımları deneyebilirsiniz, ama Ağustos 2026’dan beri ücretli planlarda da “ERROR_BAD_MODEL_NAME” hataları bildirildi; çalışacağını garanti edemeyiz.',
    en: 'Honest answer: on the free plan of Cursor the agent and edit modes do not work with your own (custom) model; this is by design. The setting where you can type your own OpenAI key and address only helps on a paid Cursor plan. There is no workaround or trick. If you do not want to buy a plan, use one of the free programs below. If you already have a paid plan you can try the steps below, but since August 2026 “ERROR_BAD_MODEL_NAME” errors have been reported even on paid plans; we cannot promise it works.',
  },
  alternatives: ['zed', 'opencode'],
  variants: [
    {
      id: 'paid',
      label: {
        tr: 'Ücretli plan (OpenAI anahtarı)',
        en: 'Paid plan (OpenAI key)',
      },
      intro: {
        tr: 'Yalnızca ücretli bir Cursor planınız varsa. Cursor yalnızca OpenAI uyumlu adresi destekler; Anthropic adresi için bir ayar bulamadık. Bu ayar “Tab” tamamlamasını etkilemez.',
        en: 'Only if you have a paid Cursor plan. Cursor only supports the OpenAI-compatible address; we found no setting for the Anthropic address. This setting does not affect “Tab” completion.',
      },
      steps: [
        {
          unverified: true,
          text: {
            tr: 'Cursor’da Ayarlar penceresini açın (Windows ve Linux’ta Ctrl + Shift + J, macOS’ta Cmd + Shift + J) ve “Models” bölümüne gidin.',
            en: 'In Cursor open the settings window (Ctrl + Shift + J on Windows and Linux, Cmd + Shift + J on macOS) and go to the “Models” section.',
          },
          see: {
            tr: 'Model listesi ve “OpenAI API Key” alanı. Düğme adları sürüme göre değişebilir.',
            en: 'A model list and an “OpenAI API Key” field. Button names may differ per version.',
          },
        },
        {
          unverified: true,
          text: {
            tr: '“OpenAI API Key” seçeneğini açın ve anahtarınızı girin. BURAYA_ANAHTARINIZ yerine 3. adımda kopyaladığınız anahtarı yazın.',
            en: 'Turn on the “OpenAI API Key” option and enter your key. Replace BURAYA_ANAHTARINIZ with the key you copied in step 3.',
          },
          blocks: [
            block('config', 'text', `OpenAI API Key: ${KEY_PLACEHOLDER}`),
          ],
        },
        {
          unverified: true,
          text: {
            tr: '“Override OpenAI Base URL” seçeneğini açın ve şu adresi yapıştırın.',
            en: 'Turn on the “Override OpenAI Base URL” option and paste this address.',
          },
          blocks: [block('settings', 'text', OPENAI_BASE_URL)],
          note: {
            tr: 'Bu adres, Cursor’un tüm OpenAI türü isteklerinde kullanılır.',
            en: 'Cursor uses this address for all its OpenAI-type requests.',
          },
        },
        {
          unverified: true,
          text: {
            tr: '“Add Custom Model” ile model adını tam olarak yazarak ekleyin, sonra sohbette bu modeli seçin.',
            en: 'Add the model with “Add Custom Model”, typing the name exactly, then choose this model in the chat.',
          },
          blocks: [block('settings', 'text', MODEL_DEFAULT)],
        },
      ],
    },
  ],
  works: [
    {
      tr: 'Ücretli planda: sohbette seçtiğiniz özel model yanıt verir ve “ERROR_BAD_MODEL_NAME” hatası görünmez.',
      en: 'On a paid plan: the custom model you picked answers in the chat and no “ERROR_BAD_MODEL_NAME” error appears.',
    },
    {
      tr: 'Ücretsiz planda bu çalışmaz; bu bir hata değil, planın sınırıdır.',
      en: 'On the free plan this does not work; that is not a mistake of yours but a limit of the plan.',
    },
  ],
  problems: [
    {
      message: {
        tr: 'ERROR_BAD_MODEL_NAME',
        en: 'ERROR_BAD_MODEL_NAME',
      },
      cause: {
        tr: 'Cursor özel model adını kabul etmiyor; Ağustos 2026’dan beri ücretli planlarda da bildirildi.',
        en: 'Cursor does not accept the custom model name; reported even on paid plans since August 2026.',
      },
      fix: {
        tr: 'Bilinen bir çözüm yok. Zed ya da OpenCode sekmesini kullanın.',
        en: 'There is no known fix. Use the Zed or OpenCode tab.',
      },
    },
    {
      message: {
        tr: 'Özel model ücretsiz planda seçilemiyor',
        en: 'The custom model cannot be selected on the free plan',
      },
      cause: {
        tr: 'Ücretsiz planda ajan ve düzenleme kipleri özel modellere kapalıdır.',
        en: 'On the free plan the agent and edit modes are closed to custom models.',
      },
      fix: {
        tr: 'Ücretli plana geçin ya da Zed ya da OpenCode’u kullanın.',
        en: 'Switch to a paid plan or use Zed or OpenCode.',
      },
    },
  ],
}

const antigravity: ToolGuide = {
  id: 'antigravity',
  name: 'Google Antigravity',
  status: 'unsupported',
  protocol: 'none',
  testedWith: {
    tr: 'Antigravity IDE 2.5.5 ve CLI 1.4.4, belgeler, 22 Ekim 2026’da incelendi',
    en: 'Antigravity IDE 2.5.5 and CLI 1.4.4, documentation, researched 22 Oct 2026',
  },
  summary: {
    tr: 'Antigravity, Google’ın kodlama editörüdür. Yalnızca Google’ın kendi hesabı ve modelleriyle çalışır.',
    en: 'Antigravity is the coding editor of Google. It only works with the own account and models of Google.',
  },
  limit: {
    tr: 'Dürüst cevap: Antigravity ile HisarWeb AI kullanılamaz. Programda kendi anahtarınızı ya da kendi adresinizi (OpenAI ya da Anthropic uyumlu) girebileceğiniz bir yer yoktur. Giriş için bir Google hesabı gerekir ve ücretsiz planda yalnızca Google’ın Gemini modeli vardır. Bunun için bir çözüm yolu ya da hile yok ve biz de bir tane göstermeyeceğiz. Aşağıdaki ücretsiz programlardan birini kullanın: onlar HisarWeb AI ile çalışır.',
    en: 'Honest answer: HisarWeb AI cannot be used with Antigravity. The program has no place where you can enter your own key or your own address (OpenAI or Anthropic compatible). Signing in needs a Google account, and the free plan only has the Gemini model of Google. There is no workaround or trick, and we will not show one. Use one of the free programs below: they work with HisarWeb AI.',
  },
  alternatives: ['zed', 'opencode'],
  variants: [],
  works: [],
  problems: [],
}

export const TOOL_GUIDES_C: readonly ToolGuide[] = [cursor, antigravity]
