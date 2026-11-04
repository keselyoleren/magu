import { get, set } from 'idb-keyval'
import { api } from './api'

export interface CatalogItem {
  id: number
  barcode: string
  name: string
  price: number
}

interface Stored {
  cursor: string
  items: CatalogItem[]
}

interface Change extends CatalogItem {
  is_active: boolean
  updated_at: string
}

const KEY = 'catalog'
const byId = new Map<number, CatalogItem>()
const byBarcode = new Map<string, CatalogItem>()
const searchNames = new Map<number, string>()
let cursor = ''
let persisted = false

export const catalog = $state({ ready: false, count: 0, error: '' })

function remove(id: number) {
  const old = byId.get(id)
  if (!old) return
  byId.delete(id)
  searchNames.delete(id)
  if (byBarcode.get(old.barcode) === old) byBarcode.delete(old.barcode)
}

function put({ id, barcode, name, price }: CatalogItem) {
  remove(id)
  const item = { id, barcode, name, price }
  byId.set(id, item)
  searchNames.set(id, normalize(name))
  if (barcode) byBarcode.set(barcode, item)
}

export async function loadCatalog() {
  try {
    if (!catalog.ready) {
      const saved = await get<Stored>(KEY).catch(() => undefined)
      persisted = saved !== undefined
      const stored = saved ?? (await api<Stored>('/api/catalog'))
      stored.items.forEach(put)
      cursor = stored.cursor
      catalog.count = byId.size
      catalog.ready = true
    }
    await syncCatalog()
    catalog.error = ''
  } catch (err) {
    catalog.error = (err as Error).message
  }
}

// Mulai 10 detik sebelum cursor supaya perubahan yang commit-nya terlambat tidak terlewat;
// menerapkan baris yang sama dua kali tidak berbahaya.
async function syncCatalog(retried = false) {
  let since = new Date(new Date(cursor).getTime() - 10_000).toISOString()
  let afterId = 0
  let serverNow = ''
  let changed = 0
  let activeCount = 0
  for (;;) {
    const res = await api<{ items: Change[]; has_more: boolean; now: string; active_count: number }>(
      `/api/catalog/changes?since=${encodeURIComponent(since)}&after_id=${afterId}`,
    )
    serverNow ||= res.now
    activeCount = res.active_count
    for (const c of res.items) c.is_active ? put(c) : remove(c.id)
    changed += res.items.length
    const last = res.items.at(-1)
    if (!res.has_more || !last) break
    since = last.updated_at
    afterId = last.id
  }
  // Produk yang dihapus tidak ikut delta; jumlah yang tidak cocok berarti katalog lokal harus diambil ulang.
  if (activeCount !== byId.size && !retried) {
    const fresh = await api<Stored>('/api/catalog')
    byId.clear()
    byBarcode.clear()
    searchNames.clear()
    fresh.items.forEach(put)
    cursor = fresh.cursor
    persisted = false
    return syncCatalog(true)
  }
  cursor = serverNow
  catalog.count = byId.size
  if (changed > 0 || !persisted) {
    await set(KEY, { cursor, items: [...byId.values()] }).catch(() => {})
    persisted = true
  }
}

// Barcode toko memakai huruf besar; scanner dengan Caps Lock menyala mengirim huruf kecil.
export const findByBarcode = (barcode: string) => byBarcode.get(barcode) ?? byBarcode.get(barcode.toUpperCase())

// Tanda baca dianggap spasi supaya "teh-botol", "1.5L", dan "1,5 L" bisa saling ketemu.
const normalize = (text: string) => ' ' + text.toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim()

// Peringkat kecil = lebih relevan. Enter di kasir langsung menambahkan hasil pertama, jadi urutan menentukan ketepatan.
// Awalan barcode hanya dihitung bila ada angka: barcode toko diawali kode member, jadi "sa" akan menarik
// semua barang member berkode SA ke atas nama yang cocok.
function rank(item: CatalogItem, name: string, phrase: string, words: string[], code: string, hasDigit: boolean) {
  if (code && item.barcode === code) return 0
  if (words.length > 0 && name.startsWith(phrase)) return 1
  if (hasDigit && item.barcode.startsWith(code)) return 2
  // "12" untuk SA00012: nomor urut di ujung barcode toko, nol di depannya boleh tidak diketik.
  const suffix = hasDigit && item.barcode.endsWith(code)
  if (suffix && /(^|[^0-9])0*$/.test(item.barcode.slice(0, -code.length))) return 2
  if (words.length === 0) return suffix ? 6 : -1
  let substring = false
  for (const w of words) {
    if (name.includes(w)) continue
    if (!name.includes(w.slice(1))) return suffix ? 6 : -1
    substring = true
  }
  if (!substring) return name.includes(phrase) ? 3 : 4
  return 5
}

export function searchCatalog(text: string, limit = 8) {
  const phrase = normalize(text)
  const code = text.trim().toUpperCase()
  if (phrase === ' ' && !code) return []
  const words = phrase.split(' ').filter(Boolean).map((w) => ' ' + w)
  const hasDigit = /[0-9]/.test(code)
  const found: { item: CatalogItem; rank: number; name: string }[] = []
  for (const item of byId.values()) {
    const name = searchNames.get(item.id)!
    const r = rank(item, name, phrase, words, code, hasDigit)
    if (r >= 0) found.push({ item, rank: r, name })
  }
  found.sort((a, b) => a.rank - b.rank || a.name.length - b.name.length || a.name.localeCompare(b.name))
  return found.slice(0, limit).map((f) => f.item)
}
