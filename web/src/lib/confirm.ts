import { reactive } from 'vue'

export const confirmState = reactive({
  open: false,
  title: 'Are you sure?',
  message: '',
  confirmLabel: 'Confirm',
  cancelLabel: 'Cancel',
  resolve: null as ((value: boolean) => void) | null,
})

export function confirm(opts: {
  title?: string
  message?: string
  confirmLabel?: string
  cancelLabel?: string
} = {}) {
  return new Promise<boolean>((resolve) => {
    confirmState.title = opts.title || 'Are you sure?'
    confirmState.message = opts.message || ''
    confirmState.confirmLabel = opts.confirmLabel || 'Confirm'
    confirmState.cancelLabel = opts.cancelLabel || 'Cancel'
    confirmState.resolve = resolve
    confirmState.open = true
  })
}

function finish(result: boolean) {
  confirmState.open = false
  const resolve = confirmState.resolve
  confirmState.resolve = null
  if (resolve) resolve(result)
}

export function confirmAccept() {
  finish(true)
}

export function confirmCancel() {
  finish(false)
}
