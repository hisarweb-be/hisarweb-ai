import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

/**
 * A relay key that is stored only as a hash (HW_TOKEN_KEY_HASH_ONLY, G2 part B)
 * comes back once, in the create response. This dialog is the one place the
 * user can copy it; closing it drops the keys from memory.
 */
export function KeyOnceDialog(props: { keys: string[]; onClose: () => void }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(true)
  const close = () => {
    setOpen(false)
    props.onClose()
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && close()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Copy your API key now')}</DialogTitle>
          <DialogDescription>
            {t(
              'This key is shown only once and is not stored in readable form. Copy it and keep it safe.'
            )}
          </DialogDescription>
        </DialogHeader>
        <ul className='grid gap-2'>
          {props.keys.map((key) => (
            <li key={key} className='flex items-center gap-2'>
              <code
                data-testid='hw-key-once-value'
                className='bg-muted min-w-0 flex-1 rounded px-2 py-1 text-sm break-all'
              >
                {key}
              </code>
              <CopyButton value={key} aria-label={t('Copy API key')} />
            </li>
          ))}
        </ul>
        <DialogFooter>
          <Button onClick={close}>
            {t('I have copied it')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
