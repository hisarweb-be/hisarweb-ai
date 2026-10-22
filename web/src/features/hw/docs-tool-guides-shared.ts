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
/** Steps that several program guides share. */
import {
  block,
  KEY_ENV_NAME,
  KEY_PLACEHOLDER,
  type GuideStep,
} from './docs-tool-guides-base'

/** Opens a terminal on each system. Used by every program that runs in one. */
export const OPEN_TERMINAL_STEPS: readonly GuideStep[] = [
  {
    only: ['windows'],
    text: {
      tr: 'Başlat menüsünü açın, PowerShell yazın ve “Windows PowerShell” uygulamasını açın. Bu pencereye “terminal” denir: bilgisayara yazıyla komut verdiğiniz pencere.',
      en: 'Open the Start menu, type PowerShell and open “Windows PowerShell”. This window is called a “terminal”: the window where you give the computer commands by typing.',
    },
    see: {
      tr: 'Koyu mavi ya da siyah bir pencere ve yanıp sönen bir imleç.',
      en: 'A dark blue or black window with a blinking cursor.',
    },
  },
  {
    only: ['macos'],
    text: {
      tr: 'Cmd + Boşluk tuşlarına basın, Terminal yazın ve Enter’a basın. “Terminal”, bilgisayara yazıyla komut verdiğiniz penceredir.',
      en: 'Press Cmd + Space, type Terminal and press Enter. The “terminal” is the window where you give the computer commands by typing.',
    },
    see: {
      tr: 'Beyaz ya da siyah bir pencere ve yanıp sönen bir imleç.',
      en: 'A white or black window with a blinking cursor.',
    },
  },
  {
    only: ['linux'],
    text: {
      tr: 'Ctrl + Alt + T tuşlarına basın ya da uygulama menüsünden Terminal’i açın. “Terminal”, bilgisayara yazıyla komut verdiğiniz penceredir.',
      en: 'Press Ctrl + Alt + T, or open Terminal from the application menu. The “terminal” is the window where you give the computer commands by typing.',
    },
    see: {
      tr: 'Bir pencere ve yanıp sönen bir imleç.',
      en: 'A window with a blinking cursor.',
    },
  },
]

/**
 * Stores the key in the environment variable `HISARWEB_API_KEY`. An
 * “environment variable” is a named note that Windows, macOS or Linux keeps
 * for every program you start; the program reads the key from it.
 */
export const KEY_ENV_STEPS: readonly GuideStep[] = [
  {
    only: ['windows'],
    text: {
      tr: 'Anahtarınızı bir “ortam değişkeni”ne kaydedin. Ortam değişkeni, Windows’un başlattığınız her programa verdiği adlandırılmış bir nottur; program anahtarı oradan okur. PowerShell’e şunu yapıştırıp Enter’a basın ve BURAYA_ANAHTARINIZ yerine 3. adımda kopyaladığınız anahtarı yazın.',
      en: 'Store your key in an “environment variable”. An environment variable is a named note that Windows gives to every program you start; the program reads the key from it. Paste this into PowerShell, press Enter, and replace BURAYA_ANAHTARINIZ with the key you copied in step 3.',
    },
    blocks: [
      block(
        'config',
        'powershell',
        `setx ${KEY_ENV_NAME} "${KEY_PLACEHOLDER}"`
      ),
    ],
    see: {
      tr: 'SUCCESS: Specified value was saved. (Değer kaydedildi.)',
      en: 'SUCCESS: Specified value was saved.',
    },
  },
  {
    only: ['windows'],
    text: {
      tr: 'Bu PowerShell penceresini kapatın ve yeni bir tane açın. Değişken yalnızca yeni açılan pencerelerde geçerli olur.',
      en: 'Close this PowerShell window and open a new one. The variable only applies to windows opened after you set it.',
    },
  },
  {
    only: ['macos'],
    text: {
      tr: 'Anahtarınızı bir “ortam değişkeni”ne kaydedin. Ortam değişkeni, macOS’un başlattığınız her programa verdiği adlandırılmış bir nottur; program anahtarı oradan okur. Terminal’e şunu yapıştırıp Enter’a basın ve BURAYA_ANAHTARINIZ yerine 3. adımda kopyaladığınız anahtarı yazın.',
      en: 'Store your key in an “environment variable”. An environment variable is a named note that macOS gives to every program you start; the program reads the key from it. Paste this into Terminal, press Enter, and replace BURAYA_ANAHTARINIZ with the key you copied in step 3.',
    },
    blocks: [
      block(
        'config',
        'bash',
        `echo 'export ${KEY_ENV_NAME}="${KEY_PLACEHOLDER}"' >> ~/.zshrc`
      ),
    ],
  },
  {
    only: ['linux'],
    text: {
      tr: 'Anahtarınızı bir “ortam değişkeni”ne kaydedin. Ortam değişkeni, sistemin başlattığınız her programa verdiği adlandırılmış bir nottur; program anahtarı oradan okur. Terminal’e şunu yapıştırıp Enter’a basın ve BURAYA_ANAHTARINIZ yerine 3. adımda kopyaladığınız anahtarı yazın. (Bash dışında bir kabuk kullanıyorsanız ~/.bashrc yerine onun dosyasını yazın.)',
      en: 'Store your key in an “environment variable”. An environment variable is a named note that the system gives to every program you start; the program reads the key from it. Paste this into the terminal, press Enter, and replace BURAYA_ANAHTARINIZ with the key you copied in step 3. (If you use a shell other than Bash, write its own startup file instead of ~/.bashrc.)',
    },
    blocks: [
      block(
        'config',
        'bash',
        `echo 'export ${KEY_ENV_NAME}="${KEY_PLACEHOLDER}"' >> ~/.bashrc`
      ),
    ],
  },
  {
    only: ['macos', 'linux'],
    text: {
      tr: 'Terminal penceresini kapatıp yeni bir tane açın. Değişken yalnızca yeni açılan pencerelerde geçerli olur.',
      en: 'Close the terminal window and open a new one. The variable only applies to windows opened after you set it.',
    },
  },
]
