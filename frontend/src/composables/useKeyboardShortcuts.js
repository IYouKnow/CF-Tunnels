import { onMounted, onUnmounted } from 'vue'

export function useKeyboardShortcuts(handlers) {
  const onKeyDown = (e) => {
    for (const [key, fn] of Object.entries(handlers)) {
      let match = false
      if (key === 'Escape') {
        match = e.key === 'Escape'
      } else if (key === 'Ctrl+N') {
        match = e.ctrlKey && e.key === 'n'
      } else if (key === 'Ctrl+F') {
        match = e.ctrlKey && e.key === 'f'
      } else if (key === 'Ctrl+R') {
        match = e.ctrlKey && e.key === 'r'
      }
      if (match) {
        e.preventDefault()
        e.stopPropagation()
        fn(e)
        return
      }
    }
  }

  onMounted(() => document.addEventListener('keydown', onKeyDown, true))
  onUnmounted(() => document.removeEventListener('keydown', onKeyDown, true))
}
