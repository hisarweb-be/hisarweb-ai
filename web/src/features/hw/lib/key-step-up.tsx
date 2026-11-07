import { createRoot } from 'react-dom/client'

import {
  KeyStepUpDialog,
  type HwKeyResponse,
} from '@/features/hw/components/key-step-up-dialog'

/**
 * Posts to a key route and asks the user for the step-up first. Resolves with
 * the server response, or a failed response when the user cancels.
 */
export function hwRequestKeyWithStepUp<T extends HwKeyResponse>(
  url: string,
  body: Record<string, unknown> = {},
): Promise<T> {
  return new Promise((resolve) => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    const onDone = (result: HwKeyResponse) => {
      resolve(result as T)
      // Let the close animation run before removing the host.
      setTimeout(() => {
        root.unmount()
        host.remove()
      }, 300)
    }
    root.render(<KeyStepUpDialog url={url} body={body} onDone={onDone} />)
  })
}
