import { onMounted, onUnmounted } from 'vue'

export function usePolling(
  fn: () => void | Promise<void>,
  intervalMs: number,
  options: { pauseWhenHidden?: boolean; immediate?: boolean } = {},
) {
  const { pauseWhenHidden = true, immediate = true } = options
  let timer: ReturnType<typeof setInterval> | null = null

  async function run() {
    if (pauseWhenHidden && document.visibilityState === 'hidden') return
    await fn()
  }

  onMounted(() => {
    if (immediate) run()
    timer = setInterval(run, intervalMs)
  })

  onUnmounted(() => {
    if (timer) clearInterval(timer)
  })

  return { refresh: run }
}
