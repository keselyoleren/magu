<script lang="ts">
  import { tick } from 'svelte'
  import Icon from '../../components/Icon.svelte'
  import { api, post } from '../../lib/api'
  import { listMembers, type Member } from '../../lib/members'
  import type { Product } from './Products.svelte'

  interface Row extends Product {
    newStock: number | null
    picked: boolean
  }

  const LIMIT = 1000

  let rows = $state<Row[]>([])
  let members = $state<Member[]>([])
  let truncated = $state(false)
  let filter = $state('')
  let memberId = $state(0)
  let view = $state<'all' | 'empty' | 'minus'>('all')
  let bulkStock = $state<number | null>(null)
  let note = $state('')
  let message = $state('')
  let failed = $state(false)
  let busy = $state(false)
  let filterInput = $state<HTMLInputElement>()

  const isChanged = (r: Row) => r.newStock !== null && r.newStock !== r.stock

  const shown = $derived.by(() => {
    const q = filter.trim().toLowerCase()
    return rows.filter(
      (r) =>
        (!q || r.name.toLowerCase().includes(q) || r.barcode.toLowerCase().includes(q)) &&
        (memberId === 0 || (memberId === -1 ? r.member_id === null : r.member_id === memberId)) &&
        (view === 'all' || (view === 'empty' && r.stock === 0) || (view === 'minus' && r.stock < 0)),
    )
  })
  const picked = $derived(shown.filter((r) => r.picked))
  const targets = $derived(picked.length ? picked : shown)
  const changed = $derived(rows.filter(isChanged))

  // Baris yang sudah dimuat dipertahankan: ganti saringan tidak boleh membuang stok yang sudah diketik.
  // Saringan tetap ke server karena produk toko bisa jauh lebih banyak dari LIMIT.
  let requestNo = 0
  async function load() {
    const current = ++requestNo
    const params = new URLSearchParams({ q: filter.trim(), member_id: String(memberId), per_page: String(LIMIT) })
    const res = await api<{ items: Product[]; has_more: boolean }>(`/api/products?${params}`)
    if (current !== requestNo) return
    const known = new Set(rows.map((r) => r.id))
    for (const p of res.items) if (!known.has(p.id)) rows.push({ ...p, newStock: p.stock, picked: false })
    truncated = res.has_more
  }

  let timer = 0
  function filterChanged() {
    clearTimeout(timer)
    timer = setTimeout(load, 250)
  }

  function applyStock() {
    if (bulkStock === null) return
    for (const r of targets) r.newStock = bulkStock
  }

  function pickAll(on: boolean) {
    for (const r of shown) r.picked = on
  }

  function focusRow(id: number) {
    const input = document.getElementById(`stock-${id}`) as HTMLInputElement | null
    input?.focus()
    input?.select()
  }

  // Scan barcode lalu Enter melompat ke kotak stok barang itu; Enter di kotak stok lanjut ke baris berikutnya,
  // atau kembali ke kolom scan kalau hanya satu barang yang tampil.
  async function filterKey(e: KeyboardEvent) {
    if (e.key !== 'Enter') return
    e.preventDefault()
    clearTimeout(timer)
    await load()
    await tick()
    const text = filter.trim().toUpperCase()
    const hit = shown.find((r) => r.barcode && r.barcode.toUpperCase() === text) ?? (shown.length === 1 ? shown[0] : undefined)
    if (hit) focusRow(hit.id)
  }

  function stockKey(e: KeyboardEvent, i: number) {
    if (e.key !== 'Enter') return
    e.preventDefault()
    const next = shown[i + 1]
    if (next) focusRow(next.id)
    else filterInput?.select()
  }

  async function save() {
    busy = true
    try {
      const { updated } = await post<{ updated: number }>('/api/products/stock', {
        note,
        items: changed.map((r) => ({ id: r.id, stock: r.newStock })),
      })
      message = `Stok ${updated} produk diperbarui.`
      failed = false
      note = ''
      bulkStock = null
      rows = []
      await load()
    } catch (err) {
      message = (err as Error).message
      failed = true
    } finally {
      busy = false
    }
  }

  listMembers().then((m) => (members = m))
  load()
</script>

<div class="page-head">
  <div class="titles">
    <h1>Atur stok</h1>
    <p>
      Samakan stok banyak barang dengan isi rak, lalu simpan sekali. Untuk kiriman barang, pakai
      <a href="#/receipts/new">Terima barang</a>
    </p>
  </div>
  <input class="reason" bind:value={note} placeholder="Alasan, mis. hitung ulang rak" aria-label="Alasan perubahan stok" />
  <button class="primary" disabled={busy || changed.length === 0} onclick={save}>
    Simpan {changed.length ? `${changed.length} perubahan` : ''}
  </button>
</div>

<div class="filters">
  <div class="search">
    <Icon name="search" />
    <input bind:this={filterInput} placeholder="Saring nama, atau scan barcode" bind:value={filter} oninput={filterChanged} onkeydown={filterKey} />
  </div>
  <select bind:value={memberId} onchange={load}>
    <option value={0}>Semua pemilik</option>
    <option value={-1}>Milik toko</option>
    {#each members as m (m.id)}
      <option value={m.id}>{m.name}</option>
    {/each}
  </select>
  <select bind:value={view}>
    <option value="all">Semua stok</option>
    <option value="empty">Stok habis</option>
    <option value="minus">Stok minus</option>
  </select>
</div>

<div class="bulk">
  <span>
    {#if picked.length}
      Untuk <strong>{picked.length}</strong> produk yang dicentang:
    {:else}
      Untuk <strong>{shown.length}</strong> produk yang tampil (centang baris untuk memilih sebagian):
    {/if}
  </span>
  <input type="number" min="0" step="1" bind:value={bulkStock} placeholder="Stok" aria-label="Stok untuk produk terpilih" onkeydown={(e) => e.key === 'Enter' && applyStock()} />
  <button disabled={bulkStock === null || targets.length === 0} onclick={applyStock}>Terapkan stok</button>
</div>

{#if message}<p class:error={failed} class="note">{message}</p>{/if}
{#if truncated}<p class="note">Hanya {LIMIT} produk pertama yang ditampilkan. Pilih pemilik atau ketik nama untuk mempersempit.</p>{/if}

<div class="table-card">
  <table>
    <thead>
      <tr>
        <th class="pick">
          <input
            type="checkbox"
            checked={shown.length > 0 && picked.length === shown.length}
            onchange={(e) => pickAll(e.currentTarget.checked)}
            aria-label="Pilih semua yang tampil"
          />
        </th>
        <th>Barcode</th>
        <th>Nama</th>
        <th>Pemilik</th>
        <th class="num">Stok sekarang</th>
        <th class="num">Stok baru</th>
        <th class="num">Selisih</th>
      </tr>
    </thead>
    <tbody>
      {#each shown as r, i (r.id)}
        {@const diff = isChanged(r) ? (r.newStock ?? 0) - r.stock : 0}
        <tr class:changed={isChanged(r)}>
          <td class="pick"><input type="checkbox" bind:checked={r.picked} aria-label="Pilih {r.name}" /></td>
          <td class="code">{r.barcode || '—'}</td>
          <td class="strong">{r.name}</td>
          <td>{r.member_name || 'Toko'}</td>
          <td class="num">{r.stock}</td>
          <td class="num">
            <input id="stock-{r.id}" type="number" min="0" step="1" bind:value={r.newStock} onkeydown={(e) => stockKey(e, i)} aria-label="Stok {r.name}" />
          </td>
          <td class="num" class:neg={diff < 0} class:pos={diff > 0}>{diff > 0 ? `+${diff}` : diff || '—'}</td>
        </tr>
      {:else}
        <tr><td colspan="7" class="empty">Tidak ada produk untuk saringan ini.</td></tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .reason {
    width: 15rem;
  }
  .bulk {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-bottom: 0.9rem;
    padding: 0.7rem 1rem;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--card);
  }
  .bulk span {
    margin-right: auto;
    color: var(--muted);
  }
  .bulk strong {
    color: var(--text);
  }
  .bulk input {
    width: 9rem;
    font-family: var(--mono);
  }
  .pick {
    width: 2.2rem;
  }
  .note {
    margin: 0 0 0.9rem;
    color: var(--muted);
  }
  td input[type='number'] {
    width: 7.5rem;
    height: 30px;
    font-family: var(--mono);
    text-align: right;
  }
  tr.changed td {
    background: var(--accent-wash);
  }
  @media (max-width: 860px) {
    .page-head,
    .filters,
    .bulk {
      flex-wrap: wrap;
    }
  }
</style>
