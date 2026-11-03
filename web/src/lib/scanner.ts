// Scanner barcode USB bertingkah seperti keyboard yang mengetik sangat cepat lalu menekan Enter.
// Jeda antar tombol di bawah maxGap ms dianggap scanner; ketikan manusia otomatis terbuang.
export function onScan(handle: (barcode: string) => void, maxGap = 50, minLength = 4) {
  let buffer = ''
  let last = 0

  const listener = (e: KeyboardEvent) => {
    const now = performance.now()
    if (now - last > maxGap) buffer = ''
    last = now

    if (e.key === 'Enter') {
      if (buffer.length >= minLength) {
        e.preventDefault()
        e.stopPropagation()
        // Karakter scan sudah terlanjur terketik di input yang sedang fokus (mis. kolom uang diterima): buang lagi.
        const el = document.activeElement
        if (el instanceof HTMLInputElement && el.value.endsWith(buffer)) {
          el.value = el.value.slice(0, -buffer.length)
          el.dispatchEvent(new Event('input', { bubbles: true }))
        }
        handle(buffer)
      }
      buffer = ''
    } else if (e.key.length === 1) {
      buffer += e.key
    }
  }

  window.addEventListener('keydown', listener, true)
  return () => window.removeEventListener('keydown', listener, true)
}
