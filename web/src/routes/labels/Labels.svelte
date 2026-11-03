<script lang="ts">
  import Barcode from '../../components/Barcode.svelte'
  import Icon from '../../components/Icon.svelte'
  import { api, post, put } from '../../lib/api'
  import { focusFirst } from '../../lib/focus'
  import { dateTime, rupiah } from '../../lib/format'
  import { listMembers, type Member } from '../../lib/members'
  import { routeParam } from '../../lib/route.svelte'
  import type { Product } from '../products/Products.svelte'

  interface Line {
    id: number
    name: string
    barcode: string
    price: number
    qty: number
  }

  interface ReceiptSummary {
    id: number
    member_id: number | null
    member_name: string
    received_at: string
    total_qty: number
    cancelled_at: string | null
  }

  const LIMIT = 1000

  let lines = $state<Line[]>([])
  let members = $state<Member[]>([])
  let receipts = $state<ReceiptSummary[]>([])
  let items = $state<Product[]>([])
  let hasMore = $state(false)
  let memberId = $state(0)
  let query = $state('')
  let message = $state('')
  let failed = $state(false)
  let scanInput = $state<HTMLInputElement>()

  const printable = $derived(lines.filter((l) => l.barcode && l.qty > 0))
  const missing = $derived(lines.filter((l) => !l.barcode).length)
  const count = $derived(printable.reduce((sum, l) => sum + l.qty, 0))
  const sheet = $derived(printable.flatMap((l) => Array.from({ length: l.qty }, () => l)))
  const withStock = $derived(items.filter((p) => p.barcode && p.stock > 0))
  const stockTotal = $derived(withStock.reduce((sum, p) => sum + p.stock, 0))
  const stockNoBarcode = $derived(items.filter((p) => !p.barcode && p.stock > 0).length)

  function tell(text: string, isError = false) {
    message = text
    failed = isError
  }

  let requestNo = 0
  async function load() {
    const current = ++requestNo
    const params = new URLSearchParams({ q: query.trim(), member_id: String(memberId), per_page: String(LIMIT) })
    const res = await api<{ items: Product[]; has_more: boolean }>(`/api/products?${params}`)
    if (current !== requestNo) return
    items = res.items.filter((p) => p.is_active)
    hasMore = res.has_more
  }

  let timer = 0
  function queryChanged() {
    clearTimeout(timer)
    timer = setTimeout(load, 250)
  }

  function add(p: { id: number; barcode: string; name: string; price: number }, qty = 1) {
    const line = lines.find((l) => l.id === p.id)
    if (line) line.qty += qty
    else lines.push({ id: p.id, barcode: p.barcode, name: p.name, price: p.price, qty })
  }

  function setQty(p: Product, qty: number) {
    const line = lines.find((l) => l.id === p.id)
    if (!(qty > 0)) lines = lines.filter((l) => l.id !== p.id)
    else if (line) line.qty = qty
    else add(p, qty)
  }

  // Berguna untuk barang yang belum pernah berlabel: satu label untuk setiap barang yang ada di rak.
  // Setiap label digambar di halaman cetak, jadi jumlah ribuan membuat browser berat dan perlu dikonfirmasi.
  function fillFromStock() {
    if (stockTotal > 1000 && !confirm(`Siapkan ${rupiah(stockTotal)} label? Halaman bisa melambat sebentar.`)) return
    for (const p of withStock) setQty(p, p.stock)
    tell(stockNoBarcode > 0 ? `${stockNoBarcode} produk berstok belum punya barcode, jadi belum ikut. Buat barcodenya dulu.` : '')
  }

  // Scanner mengetik barcode lalu Enter: barcode yang cocok menambah 1 label dan kolomnya kosong lagi untuk scan berikutnya.
  async function lookup(e: KeyboardEvent) {
    if (e.key !== 'Enter') return
    e.preventDefault()
    const text = query.trim()
    if (!text) return
    tell('')
    clearTimeout(timer)
    const found = (await api<{ items: Product[] }>(`/api/products?q=${encodeURIComponent(text)}`)).items
    const exact = found.find((p) => p.barcode === text || p.barcode === text.toUpperCase()) ?? (found.length === 1 ? found[0] : undefined)
    if (exact) {
      add(exact)
      query = ''
    } else if (found.length === 0) {
      tell(`"${text}" tidak ditemukan.`, true)
      scanInput?.select()
    }
    load()
  }

  async function makeBarcode(id: number) {
    tell('')
    try {
      const p = await api<Product>(`/api/products/${id}`)
      const { barcode } = await post<{ barcode: string }>('/api/products/barcode', { member_id: p.member_id })
      await put(`/api/products/${id}`, { ...p, barcode })
      for (const row of [...items, ...lines]) if (row.id === id) row.barcode = barcode
    } catch (err) {
      tell((err as Error).message, true)
    }
  }

  async function fillFromReceipt(id: string) {
    if (!id) return
    const r = await api<{ items: { product_id: number; barcode: string; name: string; price: number; qty: number }[] }>(`/api/receipts/${id}`)
    lines = []
    r.items.forEach((i) => add({ id: i.product_id, ...i }, i.qty))
  }

  // '#/labels/receipt/12' mengisi daftar dari satu penerimaan barang, sejumlah yang diterima; '#/labels/product/5' dari satu produk, sejumlah stoknya.
  if (routeParam() === 'receipt') fillFromReceipt(routeParam(1))
  if (routeParam() === 'product') api<Product>(`/api/products/${routeParam(1)}`).then((p) => add(p, Math.max(p.stock, 1)))

  listMembers().then((m) => (members = m))
  api<ReceiptSummary[]>('/api/receipts').then((r) => (receipts = r.filter((x) => !x.cancelled_at).slice(0, 10)))
  load()
</script>

{#snippet labelFace(label: Line)}
  <div class="label">
    <span class="name">{label.name}</span>
    <div class="bars"><Barcode value={label.barcode} /></div>
    <div class="foot">
      <span class="code">{label.barcode}</span>
      <span class="price">Rp {rupiah(label.price)}</span>
    </div>
  </div>
{/snippet}

<div class="no-print">
  <div class="page-head">
    <div class="titles">
      <h1>Cetak label</h1>
      <p>Isi jumlah label di samping barang, atau pakai Isi sesuai stok, lalu cetak</p>
    </div>
    <select class="from-receipt" aria-label="Ambil dari penerimaan" onchange={(e) => { fillFromReceipt(e.currentTarget.value); e.currentTarget.value = '' }}>
      <option value="">Ambil dari penerimaan…</option>
      {#each receipts as r (r.id)}
        <option value={r.id}>#{r.id} · {r.member_id ? r.member_name : 'Toko'} · {r.total_qty} pcs · {dateTime(r.received_at)}</option>
      {/each}
    </select>
  </div>

  <div class="layout">
    <div class="form-card" use:focusFirst>
      <div class="form-body">
        <div class="pair">
          <label>
            Cari atau scan
            <input bind:this={scanInput} bind:value={query} oninput={queryChanged} onkeydown={lookup} placeholder="Ketik nama, atau scan barcode" />
          </label>
          <label>
            Pemilik barang
            <select bind:value={memberId} onchange={load}>
              <option value={0}>Semua pemilik</option>
              <option value={-1}>Barang toko</option>
              {#each members as m (m.id)}
                <option value={m.id}>{m.name}</option>
              {/each}
            </select>
          </label>
        </div>

        {#if message}<p class:error={failed} class="muted">{message}</p>{/if}

        <div class="list-head">
          <span class="section">{items.length} produk</span>
          <button type="button" disabled={stockTotal === 0} onclick={fillFromStock} title="Jumlah label disamakan dengan stok tiap barang yang tampil">
            Isi sesuai stok{stockTotal ? ` (${rupiah(stockTotal)} label)` : ''}
          </button>
        </div>

        <div class="list">
          {#each items as p (p.id)}
            {@const line = lines.find((l) => l.id === p.id)}
            <div class="row" class:on={!!line}>
              <span class="info">
                <strong>{p.name}</strong>
                <small>{p.barcode || 'belum punya barcode'} · stok {p.stock} · Rp {rupiah(p.price)}</small>
              </span>
              {#if p.barcode}
                <input
                  type="number"
                  min="0"
                  step="1"
                  value={line?.qty ?? null}
                  oninput={(e) => setQty(p, e.currentTarget.valueAsNumber)}
                  placeholder="0"
                  aria-label="Jumlah label {p.name}"
                />
              {:else}
                <button type="button" onclick={() => makeBarcode(p.id)}>Buat barcode</button>
              {/if}
            </div>
          {:else}
            <p class="muted empty">Tidak ada produk yang cocok.</p>
          {/each}
        </div>
        {#if hasMore}<p class="muted">Hanya {LIMIT} produk pertama yang ditampilkan. Ketik nama atau pilih pemilik untuk mempersempit.</p>{/if}
      </div>
    </div>

    <div class="form-card side">
      <div class="form-body">
        <p class="section first">{count} label · {printable.length} produk</p>
        {#if printable[0]}
          <div class="preview">{@render labelFace(printable[0])}</div>
        {/if}
        {#each lines as line, i (line.id)}
          <div class="line">
            <div>
              <strong>{line.name}</strong>
              {#if !line.barcode}
                <button type="button" class="link" onclick={() => makeBarcode(line.id)}>Buat barcode</button>
              {/if}
            </div>
            <input type="number" bind:value={line.qty} min="1" step="1" aria-label="Jumlah label {line.name}" />
            <button type="button" class="link" onclick={() => lines.splice(i, 1)} aria-label="Hapus {line.name}">
              <Icon name="trash" size={16} />
            </button>
          </div>
        {:else}
          <p class="muted">Belum ada label. Isi jumlah di daftar barang.</p>
        {/each}
        {#if missing > 0}<p class="muted">{missing} produk tanpa barcode dilewati.</p>{/if}
      </div>
      <div class="form-actions">
        <button class="primary" disabled={count === 0} onclick={() => window.print()}><Icon name="printer" /> Cetak {count || ''} label</button>
      </div>
      <p class="paper">Kertas label 50 × 20 mm. Pilih printer label di jendela cetak.</p>
    </div>
  </div>
</div>

<!-- @page di <style> komponen ikut ke CSS global dan mengubah ukuran cetak layar lain; di head hanya aktif selama layar ini terbuka.
     Ukuran kertas mengikuti driver: size 50mm 20mm membuat Chrome memutar label di kertas driver yang tegak.
     size: landscape mengunci orientasi dan menyembunyikan pilihan Layout, yang kalau diganti membalik kertas lebar menjadi 20 x 50 mm. -->
<svelte:head>
  {@html '<style>@page { size: landscape; margin: 0 }</style>'}
</svelte:head>

<div class="print-only sheet">
  {#each sheet as label}
    {@render labelFace(label)}
  {/each}
</div>

<style>
  .from-receipt {
    width: auto;
    max-width: 320px;
  }
  .layout {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 320px;
    gap: 1rem;
    align-items: start;
  }
  .form-card {
    max-width: none;
  }
  .side {
    position: sticky;
    top: 1rem;
  }
  .side .form-body {
    max-height: 60vh;
    overflow-y: auto;
  }
  .side .form-actions .primary {
    width: 100%;
  }
  .list-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.6rem;
    min-height: 36px;
  }
  .section {
    margin: 0;
    padding: 0;
    border: 0;
  }
  .list {
    max-height: 58vh;
    min-height: 120px;
    overflow-y: auto;
    border: 1px solid var(--line);
    border-radius: 8px;
  }
  .row {
    display: grid;
    grid-template-columns: 1fr 7rem;
    align-items: center;
    gap: 0.8rem;
    padding: 0.5rem 0.8rem;
    border-bottom: 1px solid var(--line);
  }
  .row:last-child {
    border-bottom: 0;
  }
  .row.on {
    background: var(--accent-wash);
    box-shadow: inset 3px 0 0 var(--accent);
  }
  .row input,
  .line input {
    height: 32px;
    font-family: var(--mono);
    text-align: right;
  }
  .row button {
    height: 32px;
    padding: 0 0.5rem;
    font-size: 0.86rem;
  }
  .info {
    display: grid;
    min-width: 0;
  }
  .info strong,
  .line strong {
    overflow: hidden;
    font-weight: 500;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  small {
    font-family: var(--mono);
    font-size: 0.78rem;
    color: var(--muted);
  }
  .preview {
    display: grid;
    justify-items: center;
    padding: 0.8rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: #fff;
  }
  .preview .label {
    border: 1px dashed var(--line-strong);
    zoom: 1.35;
  }
  .line {
    display: grid;
    grid-template-columns: 1fr 4.5rem auto;
    align-items: center;
    gap: 0.5rem;
  }
  .line div {
    display: grid;
    justify-items: start;
    min-width: 0;
  }
  .line div strong {
    max-width: 100%;
  }
  .empty {
    margin: 0;
    padding: 1.5rem;
    text-align: center;
  }
  p.muted {
    margin: 0;
  }
  .paper {
    margin: 0;
    padding: 0 1.5rem 0.9rem;
    background: #faf7ef;
    border-radius: 0 0 10px 10px;
    color: var(--muted);
    font-size: 0.8rem;
    text-align: center;
  }
  @media (max-width: 860px) {
    .layout {
      grid-template-columns: 1fr;
    }
    .side {
      position: static;
    }
    .page-head {
      flex-wrap: wrap;
    }
  }

  .label {
    box-sizing: border-box;
    display: grid;
    grid-template-rows: auto 1fr auto;
    gap: 0.5mm;
    width: 50mm;
    /* Sedikit di bawah 20 mm: Chrome membulatkan tinggi halaman ke bawah, dan label setinggi kertas tumpah menjadi halaman kosong. */
    height: 19.8mm;
    /* Di tengah, karena selama kertas driver lebih lebar dari 50 mm, pojok kiri bisa berada di luar roll label. */
    margin: 0 auto;
    padding: 1.2mm 2mm;
    overflow: hidden;
    break-after: page;
    font-family: var(--sans);
    line-height: 1.1;
    color: #000;
  }
  .label:last-child {
    break-after: auto;
  }
  .name {
    overflow: hidden;
    font-size: 7pt;
    font-weight: 600;
    text-align: center;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .bars {
    min-height: 0;
  }
  .foot {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 1mm;
  }
  .code {
    font-family: var(--mono);
    font-size: 6pt;
  }
  .price {
    font-size: 10pt;
    font-weight: 600;
    white-space: nowrap;
  }
</style>
