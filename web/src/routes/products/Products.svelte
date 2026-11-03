<script lang="ts" module>
  export interface Product {
    id: number
    member_id: number | null
    member_name: string
    barcode: string
    name: string
    price: number
    commission_bps: number | null
    stock: number
    is_active: boolean
  }
</script>

<script lang="ts">
  import Drawer from '../../components/Drawer.svelte'
  import Icon from '../../components/Icon.svelte'
  import { api, ApiError } from '../../lib/api'
  import { bpsToPercent, rupiah } from '../../lib/format'
  import { untrack } from 'svelte'
  import { listMembers, type Member } from '../../lib/members'
  import { routeParam } from '../../lib/route.svelte'
  import ProductForm from './ProductForm.svelte'
  import StockCard from './StockCard.svelte'
  import StockForm from './StockForm.svelte'

  const param = $derived(routeParam())

  let members = $state<Member[]>([])
  let items = $state<Product[]>([])
  let hasMore = $state(false)
  let search = $state('')
  let memberId = $state(0)
  let page = $state(0)
  let stockOf = $state<Product | null>(null)
  let notice = $state('')
  let problems = $state<string[]>([])
  let fileInput = $state<HTMLInputElement>()

  let requestNo = 0
  async function load() {
    const current = ++requestNo
    const params = new URLSearchParams({ q: search, member_id: String(memberId), page: String(page) })
    const res = await api<{ items: Product[]; has_more: boolean }>(`/api/products?${params}`)
    if (current !== requestNo) return
    items = res.items
    hasMore = res.has_more
  }

  let timer = 0
  function searchChanged() {
    clearTimeout(timer)
    timer = setTimeout(() => goTo(0), 250)
  }

  function goTo(n: number) {
    page = n
    load()
  }

  async function importFile() {
    const file = fileInput?.files?.[0]
    if (!file || !fileInput) return
    notice = 'Mengimpor…'
    problems = []
    try {
      const res = await api<{ imported: number }>('/api/products/import', {
        method: 'POST',
        headers: { 'Content-Type': 'text/csv' },
        body: file,
      })
      notice = `${rupiah(res.imported)} produk ditambahkan atau diperbarui.`
      goTo(0)
    } catch (err) {
      notice = (err as Error).message
      problems = err instanceof ApiError ? err.details : []
    } finally {
      fileInput.value = ''
    }
  }

  $effect(() => {
    void param
    listMembers().then((m) => (members = m))
    untrack(load)
  })
</script>

<div class="page-head">
  <div class="titles">
    <h1>Produk</h1>
    <p>Cari barangnya, lalu pilih Stok untuk membetulkan jumlah, Label untuk mencetak, atau Ubah untuk nama dan harga</p>
  </div>
  <input type="file" accept=".csv,text/csv" bind:this={fileInput} onchange={importFile} hidden />
  <button onclick={() => fileInput?.click()}><Icon name="upload" /> Impor CSV</button>
  <a class="button" href="#/prices"><Icon name="edit" /> Atur harga</a>
  <a class="button" href="#/stocks"><Icon name="sliders" /> Atur stok</a>
  <a class="button primary" href="#/products/new"><Icon name="plus" /> Tambah produk</a>
</div>

{#if notice}
  <div class="notice" class:failed={problems.length > 0}>
    <button class="link" onclick={() => (notice = '')} aria-label="Tutup"><Icon name="close" size={16} /></button>
    <strong>{notice}</strong>
    {#if problems.length > 0}
      <ul>
        {#each problems as p}<li>{p}</li>{/each}
      </ul>
    {/if}
    <p>
      Kolom CSV: <code>name</code>, <code>price</code> (wajib), <code>barcode</code>, <code>member_code</code>,
      <code>commission_percent</code>. Baris dengan barcode yang sudah ada memperbarui produk itu.
    </p>
  </div>
{/if}

<div class="filters">
  <div class="search">
    <Icon name="search" />
    <input placeholder="Cari nama atau scan barcode" bind:value={search} oninput={searchChanged} />
  </div>
  <select bind:value={memberId} onchange={() => goTo(0)}>
    <option value={0}>Semua pemilik</option>
    <option value={-1}>Milik toko</option>
    {#each members as m (m.id)}
      <option value={m.id}>{m.name}</option>
    {/each}
  </select>
</div>

<div class="table-card">
  <table>
    <thead>
      <tr>
        <th>Barcode</th>
        <th>Nama</th>
        <th>Pemilik</th>
        <th class="num">Harga</th>
        <th class="num">Komisi toko</th>
        <th class="num">Stok</th>
        <th></th>
      </tr>
    </thead>
    <tbody>
      {#each items as p (p.id)}
        <tr class:inactive={!p.is_active}>
          <td class="code">{p.barcode || '—'}</td>
          <td class="strong">
            {p.name}
            {#if !p.is_active}<span class="badge off">Nonaktif</span>{/if}
          </td>
          <td>
            {#if p.member_id}{p.member_name}{:else}<span class="badge store">Toko</span>{/if}
          </td>
          <td class="num">{rupiah(p.price)}</td>
          <td class="num">
            {#if p.commission_bps !== null}
              {bpsToPercent(p.commission_bps)}%
            {:else}
              <span class="muted">{p.member_id ? 'ikut member' : '—'}</span>
            {/if}
          </td>
          <td class="num"><button class="link" title="Lihat riwayat stok" onclick={() => (stockOf = p)}>{rupiah(p.stock)}</button></td>
          <td class="num actions">
            <a class="row-link" href="#/products/{p.id}/stock">Stok</a>
            <a class="row-link" href="#/labels/product/{p.id}">Label</a>
            <a class="row-link" href="#/products/{p.id}">Ubah</a>
          </td>
        </tr>
      {:else}
        <tr><td colspan="7" class="empty">Tidak ada produk.</td></tr>
      {/each}
    </tbody>
  </table>
</div>

<div class="pager">
  <button disabled={page === 0} onclick={() => goTo(page - 1)}>‹ Sebelumnya</button>
  <span>Halaman {page + 1}</span>
  <button disabled={!hasMore} onclick={() => goTo(page + 1)}>Berikutnya ›</button>
</div>

{#if stockOf}
  <Drawer title="Kartu stok" onclose={() => (stockOf = null)}>
    <StockCard productId={stockOf.id} />
    <div class="drawer-foot">
      <a class="button primary" href="#/products/{stockOf.id}/stock" onclick={() => (stockOf = null)}><Icon name="edit" /> Ubah stok</a>
    </div>
  </Drawer>
{/if}
{#if routeParam(1) === 'stock'}
  {#key param}
    <StockForm id={Number(param)} />
  {/key}
{:else if param}
  {#key param}
    <ProductForm id={param === 'new' ? null : Number(param)} />
  {/key}
{/if}

<style>
  .actions {
    font-family: var(--sans);
    white-space: nowrap;
  }
  .pager {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.8rem;
    margin-top: 0.9rem;
    font-family: var(--mono);
    font-size: 0.8rem;
    color: var(--muted);
  }
  .notice {
    position: relative;
    margin-bottom: 1rem;
    padding: 0.9rem 2.6rem 0.9rem 1.1rem;
    border: 1px solid var(--line-strong);
    border-left: 3px solid var(--accent);
    border-radius: 8px;
    background: var(--card);
  }
  .notice.failed {
    border-left-color: var(--danger);
  }
  .notice.failed strong {
    color: var(--danger);
  }
  .notice > button {
    position: absolute;
    top: 0.6rem;
    right: 0.6rem;
    color: var(--muted);
  }
  .notice ul {
    margin: 0.5rem 0 0;
    padding-left: 1.1rem;
  }
  .notice p {
    margin: 0.5rem 0 0;
    font-size: 0.86rem;
    color: var(--muted);
  }
  code {
    font-family: var(--mono);
    font-size: 0.92em;
  }
</style>
