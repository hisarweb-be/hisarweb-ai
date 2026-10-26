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
import type { TFunction } from 'i18next'

import { customerGroupLabel } from './customer-groups'

/**
 * A customer cannot pick a group, so a group name such as "paid" means nothing
 * to them. Everything that is on sale reads as "Everyone", the free group as
 * "Free"; an unknown group keeps its configured name.
 */
export function plainGroupLabel(group: string, t: TFunction): string {
  if (group === 'paid' || group === 'default' || group === 'vip') {
    return t('Everyone')
  }
  return customerGroupLabel(group, t)
}

/**
 * The average time until the first words appear, over the groups that
 * measured one. Returns `0` (shown as a dash) when nothing was measured.
 */
export function averageResponseTimeMs(
  groups: ReadonlyArray<{ avg_ttft_ms: number }>
): number {
  const measured = groups
    .map((group) => group.avg_ttft_ms)
    .filter((value) => Number.isFinite(value) && value > 0)
  if (measured.length === 0) return 0
  return measured.reduce((sum, value) => sum + value, 0) / measured.length
}

/**
 * One plain sentence per parameter description, for someone who has never
 * called an API. Keyed by the technical description the data carries.
 */
const PLAIN_PARAM_DESCRIPTIONS: Record<string, string> = {
  'Sampling temperature; lower is more deterministic':
    'How creative the answer is. A low number gives steady, predictable answers; a high number gives more varied ones.',
  'Nucleus sampling probability mass':
    'Another way to control variety. Leave it as it is unless you know you need it.',
  'Maximum number of tokens in the response':
    'The longest answer you allow, in tokens. 1,000 tokens is about 750 words.',
  'Penalises repetition of frequent tokens':
    'Higher numbers make the model repeat itself less.',
  'Encourages introducing new topics':
    'Higher numbers push the model to bring up new topics.',
  'Up to 4 strings that stop generation':
    'Up to 4 pieces of text. The model stops writing as soon as it reaches one of them.',
  'Deterministic sampling seed (best-effort)':
    'A number that makes repeated requests give very similar answers.',
  'Number of completions to generate':
    'How many different answers you want back for one request.',
  'Stream tokens via Server-Sent Events':
    'Show the answer word by word while it is written, instead of waiting for all of it.',
  'Force JSON object or schema-conforming output':
    'Make the model answer in a fixed format (JSON) that your code can read.',
  'Tool / function declarations the model may call':
    'A list of actions your app offers, such as looking up the weather, that the model may ask to use.',
  'Tool-choice policy or specific tool name':
    'Whether the model may, must or may not use those actions.',
  'Return per-token log probabilities':
    'Also return how sure the model was about each word. Mostly useful for developers.',
  'Number of top log probabilities returned per token':
    'How many alternative words to show for each word. Needs the option above.',
  'Per-token logit bias map':
    'Make specific words more or less likely. Advanced; you can ignore it.',
  'End-user identifier for abuse monitoring':
    'An optional ID for your own end user, so misuse can be traced to the right person.',
  'Controls how much the model thinks before answering':
    'How long the model thinks before it answers. More thinking is slower and costs more.',
  'Maximum tokens including hidden reasoning tokens':
    'The longest answer you allow, in tokens, counting the thinking the model does behind the scenes.',
  'Text or array of texts to embed':
    'The text, or a list of texts, to turn into numbers that a computer can compare.',
  'Truncate embeddings to this many dimensions':
    'How long the list of numbers per text should be. A shorter list is cheaper to store.',
  'Wire encoding for the embedding vectors':
    'The format the numbers come back in. Leave it as it is unless you know you need another.',
  'Text description of the desired image':
    'Describe the image you want in your own words.',
  'Output image size': 'The width and height of the picture, in pixels.',
  'Generation quality preset':
    'Standard is faster and cheaper; hd gives more detail.',
  'Aesthetic style':
    'Vivid gives bold, dramatic pictures; natural gives calmer, more realistic ones.',
  'Number of images to generate':
    'How many images you want back for one request.',
  'How to deliver the resulting image':
    'Whether you get a link to the image or the image itself inside the answer.',
  'Text description of the desired video':
    'Describe the video you want in your own words.',
  'Video length in seconds': 'How long the video lasts, in seconds.',
  'Output aspect ratio':
    'The shape of the video, for example wide (16:9) or tall (9:16).',
  'Frames per second':
    'How smooth the video looks. 24 frames per second is the same as a film.',
}

/** The translation key of the plain sentence that explains a parameter. */
export function plainParamDescriptionKey(descriptionKey: string): string {
  return PLAIN_PARAM_DESCRIPTIONS[descriptionKey] ?? descriptionKey
}

/** One token is about three quarters of a word. */
const WORDS_PER_TOKEN = 0.75

/** Words per second for a speed in tokens per second; null when unknown. */
export function wordsPerSecond(tokensPerSecond: number): number | null {
  if (!Number.isFinite(tokensPerSecond) || tokensPerSecond <= 0) return null
  return Math.max(1, Math.round(tokensPerSecond * WORDS_PER_TOKEN))
}
