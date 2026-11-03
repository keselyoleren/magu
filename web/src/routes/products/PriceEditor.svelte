<script lang="ts">
  import Icon from '../../components/Icon.svelte'
  import { api, put } from '../../lib/api'
  import { rupiah } from '../../lib/format'
  import type { Product } from './Products.svelte'

  interface Row extends Product {
    newPrice: number | null
    newActive: boolean
    picked: boolean
  }

  const LIMIT = 1000

  let rows = $state<Row[]>([])
  let truncated = $state(false)
  let filter = $state('')
  let view = $state<'all' | 'noprice' | 'inactive'>('all')
  let bulkPrice = $state<number | null>(null)
  let message = $state('')
  let failed = $state(false)
  let busy = $state(false)

  const shown = $derived.by(() => {
    const q = filter.trim().toLowerCase()
    return rows.filter(
      (r) =>
        (!q || r.name.toLowerCase().includes(q) || r.barcode.toLowerCase().includes(q)) &&
        (view === 'all' || (view === 'noprice' && r.price === 0) || (view === 'inactive' && !r.is_active)),
    )
  })
  const picked = $derived(shown.filter((r) => r.picked))
  const targets = $derived(picked.length ? picked : shown)
  const changed = $derived(rows.filter((r) => (r.newPrice ?? 0) !== r.price || r.newActive !== r.is_active))

  async function load() {
    const res = await api<{ items: Product[]; has_more: boolean }>(`/api/products?per_page=${LIMIT}`)
    rows = res.items.map((p) => ({ ...p, newPrice: p.price || null, newActive: p.is_active, picked: false }))
    truncated = res.has_more
  }

  // Mengisi harga juga mengaktifkan produknya: produk berharga yang masih nonaktif hampir selalu kelupaan.
  function applyPrice() {
    if (bulkPrice === null) return
    for (const r of targets) {
      r.newPrice = bulkPrice
      r.newActive = bulkPrice > 0
    }
  }

  function priceTyped(r: Row) {
    if ((r.newPrice ?? 0) > 0 && r.price === 0) r.newActive = true
  }

  function activateTargets() {
    for (const r of targets) if ((r.newPrice ?? 0) > 0) r.newActive = true
  }

  function pickAll(on: boolean) {
    for (const r of shown) r.picked = on
  }

  async function save() {
    busy = true
    try {
      const { updated } = await put<{ updated: number }>(
        '/api/products/prices',
        changed.map((r) => ({ id: r.id, price: r.newPrice ?? 0, is_active: r.newActive })),
      )
      message = `${updated} produk diperbarui.`
      failed = false
      await load()
    } catch (err) {
      message = (err as Error).message
      failed = true
    } finally {
      busy = false
    }
  }

  load()
</script>

<div class="page-head">
  <div class="titles">
    <h1>Atur harga</h1>
    <p>Ubah harga dan status banyak produk sekaligus, lalu simpan sekali</p>
  </div>
  <button class="primary" disabled={busy || changed.length === 0} onclick={save}>
    Simpan {changed.length ? `${changed.length} perubahan` : ''}
  </button>
</div>

<div class="filters">
  <div class="search">
    <Icon name="search" />
    <input placeholder="Saring nama, mis. hippers" bind:value={filter} />
  </div>
  <select bind:value={view}>
    <option value="all">Semua produk</option>
    <option value="noprice">Belum punya harga</option>
    <option value="inactive">Nonaktif</option>
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
  <input type="number" min="0" step="500" bind:value={bulkPrice} placeholder="Harga" aria-label="Harga untuk produk terpilih" onkeydown={(e) => e.key === 'Enter' && applyPrice()} />
  <button disabled={bulkPrice === null || targets.length === 0} onclick={applyPrice}>Terapkan harga</button>
  <button disabled={targets.length === 0} onclick={activateTargets}>Aktifkan yang berharga</button>
</div>

{#if message}<p class:error={failed} class="note">{message}</p>{/if}
{#if truncated}<p class="note">Hanya {LIMIT} produk pertama yang ditampilkan.</p>{/if}

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
        <th class="num">Stok</th>
        <th class="num">Harga sekarang</th>
        <th class="num">Harga baru</th>
        <th>Aktif</th>
      </tr>
    </thead>
    <tbody>
      {#each shown as r (r.id)}
        <tr class:changed={(r.newPrice ?? 0) !== r.price || r.newActive !== r.is_active}>
          <td class="pick"><input type="checkbox" bind:checked={r.picked} aria-label="Pilih {r.name}" /></td>
          <td class="code">{r.barcode || '—'}</td>
          <td class="strong">{r.name}</td>
          <td>{r.member_name || 'Toko'}</td>
          <td class="num">{r.stock}</td>
          <td class="num">{r.price ? rupiah(r.price) : '—'}</td>
          <td class="num">
            <input type="number" min="0" step="500" bind:value={r.newPrice} oninput={() => priceTyped(r)} placeholder="0" aria-label="Harga {r.name}" />
          </td>
          <td><input type="checkbox" bind:checked={r.newActive} aria-label="Aktif {r.name}" /></td>
        </tr>
      {:else}
        <tr><td colspan="8" class="empty">Tidak ada produk untuk saringan ini.</td></tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
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
</style>
