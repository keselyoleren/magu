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
let cursor = ''
let persisted = false

export const catalog = $state({ ready: false, count: 0, error: '' })

function remove(id: number) {
  const old = byId.get(id)
  if (!old) return
  byId.delete(id)
  if (byBarcode.get(old.barcode) === old) byBarcode.delete(old.barcode)
}

function put({ id, barcode, name, price }: CatalogItem) {
  remove(id)
  const item = { id, barcode, name, price }
  byId.set(id, item)
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

export function searchCatalog(text: string, limit = 8) {
  const q = text.trim().toLowerCase()
  const found: CatalogItem[] = []
  if (!q) return found
  for (const item of byId.values()) {
    if (item.name.toLowerCase().includes(q) || item.barcode.startsWith(q)) {
      found.push(item)
      if (found.length === limit) break
    }
  }
  return found
}
