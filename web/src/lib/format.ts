const dateTimeFormat = new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium', timeStyle: 'short' })

export const dateTime = (iso: string) => dateTimeFormat.format(new Date(iso))

const dateFormat = new Intl.DateTimeFormat('id-ID', { dateStyle: 'medium' })

export const dateOnly = (iso: string) => dateFormat.format(new Date(iso))

// Tanggal kalender lokal sebagai 'YYYY-MM-DD', untuk <input type="date"> dan parameter API.
export function isoDate(d = new Date()) {
  return new Date(d.getTime() - d.getTimezoneOffset() * 60_000).toISOString().slice(0, 10)
}

export const rupiah = (n: number) => n.toLocaleString('id-ID')

export const bpsToPercent = (bps: number) => String(bps / 100).replace('.', ',')

export function percentToBps(text: string): number | null {
  const value = Number(text.trim().replace(',', '.'))
  if (text.trim() === '' || Number.isNaN(value) || value < 0 || value > 100) return null
  return Math.round(value * 100)
}
