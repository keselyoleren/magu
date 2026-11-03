// crypto.randomUUID hanya ada di https atau localhost; kasir di jaringan toko bisa saja membuka lewat http biasa.
export function uuid(): string {
  if ('randomUUID' in crypto) return crypto.randomUUID()
  const bytes = (crypto as Crypto).getRandomValues(new Uint8Array(16))
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex = [...bytes].map((b) => b.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}
