import { createRoot } from 'react-dom/client'

import { KeyOnceDialog } from '@/features/hw/components/key-once-dialog'

/** Shows freshly created keys once. Keys are passed with their `sk-` prefix. */
export function hwShowKeysOnce(keys: string[]) {
  if (keys.length === 0) return
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  const onClose = () => {
    // Let the close animation run before removing the host.
    setTimeout(() => {
      root.unmount()
      host.remove()
    }, 300)
  }
  root.render(<KeyOnceDialog keys={keys} onClose={onClose} />)
}
