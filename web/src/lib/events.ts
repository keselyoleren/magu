export interface ServerEvent {
  event: string
  data: unknown
}

// onOpen juga terpanggil setiap kali tersambung ulang: pakai untuk mengambil ulang status
// dari REST, karena event yang terkirim saat koneksi putus tidak diulang server.
export function connectEvents(onEvent: (e: ServerEvent) => void, onOpen: () => void) {
  let ws: WebSocket
  let delay = 1000
  let stopped = false

  const open = () => {
    ws = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws`)
    ws.onopen = () => {
      delay = 1000
      onOpen()
    }
    ws.onmessage = (m) => onEvent(JSON.parse(m.data))
    ws.onclose = () => {
      if (stopped) return
      setTimeout(open, delay)
      delay = Math.min(delay * 2, 15000)
    }
  }
  open()

  return () => {
    stopped = true
    ws.close()
  }
}
