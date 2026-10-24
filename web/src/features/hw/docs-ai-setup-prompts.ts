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
 * Copy-paste prompts that let Claude Code or Codex set themselves up for
 * HisarWeb AI. They live outside the text dictionaries on purpose: they hold
 * JSON and TOML with braces, which must never pass through `fillHwText`.
 *
 * The key placeholder appears exactly once per prompt, so a beginner replaces
 * one word. The settings below reuse the tested blocks of the tool guides.
 */
import { CLAUDE_SETTINGS, CODEX_PROVIDER_TABLE } from './docs-tool-guides-a'
import {
  ANTHROPIC_BASE_URL,
  KEY_ENV_NAME,
  KEY_PLACEHOLDER,
  MODEL_CODEX,
  MODEL_DEFAULT,
  MODEL_HAIKU,
  MODEL_OPUS,
  OPENAI_BASE_URL,
} from './docs-tool-guides-base'

const MODELS_URL = `${OPENAI_BASE_URL}/models`
const BUY_URL = 'https://api.hisarweb.be/satin-al'
const CLAUDE_FILE = '~/.hisarweb/claude-code-settings.json'
const CODEX_PROFILE = `model = "${MODEL_CODEX}"
model_provider = "hisarweb"`

function claudeSettings(keyMarker: string): string {
  return CLAUDE_SETTINGS.replace(KEY_PLACEHOLDER, keyMarker)
}

const CLAUDE_TR = `Claude Code’u HisarWeb AI ile çalışacak şekilde benim için kur. Kod bilmiyorum: her adımı sen yap ve sonunda bana sade Türkçeyle ne yaptığını anlat.

Bilgiler:
- API adresi: ${ANTHROPIC_BASE_URL}
- API anahtarım: ${KEY_PLACEHOLDER}
- Günlük model: ${MODEL_DEFAULT}
- Güçlü model: ${MODEL_OPUS}
- Küçük işler ve alt ajanlar: ${MODEL_HAIKU}

Kurallar:
1. Anahtarımı yalnızca aşağıdaki ayar dosyasına yaz. Başka hiçbir dosyaya ve özetine yazma.
2. ~/.claude/settings.json dosyama dokunma. Aboneliğimle giriş yaptığım normal Claude Code aynı kalsın.
3. Emin olmadığın bir şeyi uydurma. Bir adım başarısız olursa dur ve bana hata mesajını göster.

Adımlar:
1. Anahtarı test et: ${MODELS_URL} adresine "Authorization: Bearer <anahtarım>" başlığıyla bir GET isteği gönder. 401 gelirse dur ve anahtarın yanlış ya da eksik kopyalandığını söyle.
2. Gelen listede ${MODEL_DEFAULT} yoksa (ücretsiz deneme anahtarları yalnızca ${MODEL_HAIKU} çalıştırır) aşağıdaki dosyada bütün model satırlarına ${MODEL_HAIKU} yaz.
3. ${CLAUDE_FILE} dosyasını oluştur (Windows’ta %USERPROFILE%\\.hisarweb\\claude-code-settings.json). İçine tam olarak bunu yaz, <ANAHTARIM> yerine yukarıdaki anahtarımı koy:
${claudeSettings('<ANAHTARIM>')}
4. macOS ya da Linux kullanıyorsam dosyanın iznini 600 yap, böylece dosyayı yalnızca ben okuyabilirim.
5. Ayarı dene: claude --settings <dosyanın tam yolu> -p "Sadece çalışıyor yaz"
   - 401 alırsan dosyada ANTHROPIC_AUTH_TOKEN yerine ANTHROPIC_API_KEY yaz ve yeniden dene.
   - "400 Extra inputs are not permitted" alırsan "env" bölümüne "CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS": "1" satırını ekle ve yeniden dene.
   - İçinde HTML olan bir 403 alırsan dur ve bana ağımın ya da güvenlik duvarımın isteği engelliyor olabileceğini söyle.
6. Sonunda bana sade Türkçeyle şunları yaz: kurulum çalıştı mı, dosya nerede, hangi modeller ayarlandı ve HisarWeb AI ile çalışan Claude Code’u açan tek komut (claude --settings <dosyanın tam yolu>). Bir hata kaldıysa /docs sayfasındaki Claude Code rehberinin “Sık karşılaşılan sorunlar” bölümüne bakmamı söyle. Anahtarımı özette gösterme.`

const CLAUDE_EN = `Set up Claude Code for me so it works with HisarWeb AI. I do not know how to code: do every step yourself and at the end tell me in plain English what you did.

Details:
- API address: ${ANTHROPIC_BASE_URL}
- My API key: ${KEY_PLACEHOLDER}
- Everyday model: ${MODEL_DEFAULT}
- Strong model: ${MODEL_OPUS}
- Small jobs and subagents: ${MODEL_HAIKU}

Rules:
1. Write my key only into the settings file below. Do not write it into any other file or into your summary.
2. Do not touch my ~/.claude/settings.json file. The normal Claude Code I sign in to with my subscription must stay as it is.
3. Do not make up anything you are unsure about. If a step fails, stop and show me the error message.

Steps:
1. Test the key: send a GET request to ${MODELS_URL} with the header "Authorization: Bearer <my key>". If you get 401, stop and tell me the key was copied wrongly or only partly.
2. If ${MODEL_DEFAULT} is missing from the list (free trial keys only run ${MODEL_HAIKU}), write ${MODEL_HAIKU} on every model line of the file below.
3. Create the file ${CLAUDE_FILE} (on Windows %USERPROFILE%\\.hisarweb\\claude-code-settings.json). Write exactly this into it, with my key from above in place of <MY_KEY>:
${claudeSettings('<MY_KEY>')}
4. If I use macOS or Linux, set the file permission to 600 so only I can read it.
5. Test the setup: claude --settings <full path of the file> -p "Just write it works"
   - If you get 401, write ANTHROPIC_API_KEY instead of ANTHROPIC_AUTH_TOKEN in the file and try again.
   - If you get "400 Extra inputs are not permitted", add the line "CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS": "1" to the "env" part and try again.
   - If you get a 403 with HTML in it, stop and tell me my network or firewall may be blocking the request.
6. At the end write in plain English: did the setup work, where is the file, which models are set and the one command that opens Claude Code with HisarWeb AI (claude --settings <full path of the file>). If an error is left, tell me to check the “Common problems” part of the Claude Code guide on the /docs page. Do not show my key in the summary.`

const CODEX_TR = `Codex’i HisarWeb AI ile çalışacak şekilde benim için kur. Kod bilmiyorum: her adımı sen yap ve sonunda bana sade Türkçeyle ne yaptığını anlat.

Bilgiler:
- API adresi: ${OPENAI_BASE_URL}
- API anahtarım: ${KEY_PLACEHOLDER}
- Model: ${MODEL_CODEX}

Kurallar:
1. Anahtarımı yalnızca ${KEY_ENV_NAME} adlı kullanıcı ortam değişkenine yaz. Hiçbir dosyaya (config.toml dahil) ve özetine yazma.
2. ~/.codex/config.toml dosyasının en üstündeki model ve model_provider satırlarını değiştirme. Aboneliğimle çalışan normal Codex aynı kalsın.
3. Emin olmadığın bir şeyi uydurma. Bir adım başarısız olursa dur ve bana hata mesajını göster.

Adımlar:
1. Anahtarı test et: ${MODELS_URL} adresine "Authorization: Bearer <anahtarım>" başlığıyla bir GET isteği gönder. 401 gelirse dur ve anahtarın yanlış ya da eksik kopyalandığını söyle.
2. Gelen listede ${MODEL_CODEX} yoksa dur ve bana şunu söyle: ücretsiz deneme anahtarları yalnızca ${MODEL_HAIKU} çalıştırır; Codex için ${BUY_URL} sayfasından bakiye yükleyip “Ücretli” grubunda yeni bir anahtar oluşturmam gerekir.
3. Anahtarımı ${KEY_ENV_NAME} adlı kullanıcı ortam değişkeni olarak kalıcı kaydet: Windows’ta setx ile, macOS’ta ~/.zshrc dosyasında, Linux’ta ~/.bashrc dosyasında. Değişkeni şu anki terminalde de ayarla.
4. ~/.codex/config.toml dosyasında [model_providers.hisarweb] bölümü yoksa dosyanın sonuna tam olarak şunu ekle:
${CODEX_PROVIDER_TABLE}
5. ~/.codex/hisarweb.config.toml adında bir profil dosyası oluştur ve içine tam olarak şunu yaz:
${CODEX_PROFILE}
   Bir sonraki adım profilin bulunamadığını söylerse aynı iki satırı config.toml dosyasının sonuna [profiles.hisarweb] başlığının altına yaz ve yeniden dene.
6. Ayarı dene: codex exec --profile hisarweb "Sadece çalışıyor yaz"
   - 404 ve /v1/responses görürsen dur ve bana bu bağlantının henüz açık olmadığını, daha sonra yeniden denemem gerektiğini söyle.
7. Sonunda bana sade Türkçeyle şunları yaz: kurulum çalıştı mı, hangi dosyaları değiştirdin ve HisarWeb AI ile çalışan Codex’i açan tek komut (codex --profile hisarweb). Windows kullanıyorsam bu komut için yeni bir terminal açmam gerektiğini söyle. Bir hata kaldıysa /docs sayfasındaki Codex rehberinin “Sık karşılaşılan sorunlar” bölümüne bakmamı söyle. Anahtarımı özette gösterme.`

const CODEX_EN = `Set up Codex for me so it works with HisarWeb AI. I do not know how to code: do every step yourself and at the end tell me in plain English what you did.

Details:
- API address: ${OPENAI_BASE_URL}
- My API key: ${KEY_PLACEHOLDER}
- Model: ${MODEL_CODEX}

Rules:
1. Write my key only into the user environment variable ${KEY_ENV_NAME}. Do not write it into any file (config.toml included) or into your summary.
2. Do not change the model and model_provider lines at the top of ~/.codex/config.toml. The normal Codex that runs on my subscription must stay as it is.
3. Do not make up anything you are unsure about. If a step fails, stop and show me the error message.

Steps:
1. Test the key: send a GET request to ${MODELS_URL} with the header "Authorization: Bearer <my key>". If you get 401, stop and tell me the key was copied wrongly or only partly.
2. If ${MODEL_CODEX} is missing from the list, stop and tell me: free trial keys only run ${MODEL_HAIKU}; for Codex I need to add balance on ${BUY_URL} and create a new key in the “Paid” group.
3. Save my key permanently as the user environment variable ${KEY_ENV_NAME}: with setx on Windows, in ~/.zshrc on macOS, in ~/.bashrc on Linux. Also set the variable in the current terminal.
4. If ~/.codex/config.toml has no [model_providers.hisarweb] part yet, add exactly this at the end of the file:
${CODEX_PROVIDER_TABLE}
5. Create a profile file named ~/.codex/hisarweb.config.toml and write exactly this into it:
${CODEX_PROFILE}
   If the next step says the profile cannot be found, write the same two lines at the end of config.toml under a [profiles.hisarweb] heading and try again.
6. Test the setup: codex exec --profile hisarweb "Just write it works"
   - If you see 404 and /v1/responses, stop and tell me this endpoint is not open yet and I should try again later.
7. At the end write in plain English: did the setup work, which files did you change and the one command that opens Codex with HisarWeb AI (codex --profile hisarweb). If I use Windows, tell me to open a new terminal for this command. If an error is left, tell me to check the “Common problems” part of the Codex guide on the /docs page. Do not show my key in the summary.`

/** The two setup prompts per page language. */
export const AI_SETUP_PROMPTS: Readonly<
  Record<'tr' | 'en', { claude: string; codex: string }>
> = {
  tr: { claude: CLAUDE_TR, codex: CODEX_TR },
  en: { claude: CLAUDE_EN, codex: CODEX_EN },
}
