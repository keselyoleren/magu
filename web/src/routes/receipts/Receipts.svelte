<script lang="ts">
  import Drawer from '../../components/Drawer.svelte'
  import Icon from '../../components/Icon.svelte'
  import { api, post } from '../../lib/api'
  import { readDraft, writeDraft, type DocDraft } from '../../lib/docDraft'
  import { dateTime, isoDate, rupiah } from '../../lib/format'
  import { listMembers, type Member } from '../../lib/members'
  import { printGoodsReceipt } from '../../lib/receipt'
  import { routeParam } from '../../lib/route.svelte'
  import ReceiptForm from './ReceiptForm.svelte'

  interface Receipt {
    id: number
    member_id: number | null
    member_name: string
    note: string
    received_at: string
    received_by: string
    item_count: number
    total_qty: number
    cancelled_at: string | null
    cancelled_by: string
    items?: { product_id: number; barcode: string; name: string; qty: number; price: number; stock: number }[]
  }

  const DOC_LIMIT = 200

  let { kind = 'receipts' }: { kind?: 'receipts' | 'returns' } = $props()

  const isReturn = $derived(kind === 'returns')
  const text = $derived(
    isReturn
      ? { title: 'Retur ke penitip', sub: 'Barang titipan yang dikembalikan ke pemiliknya', add: 'Retur barang', one: 'Retur', many: 'Dokumen retur', flow: 'Barang keluar', empty: 'Tidak ada retur pada saringan ini.', sign: '−' }
      : { title: 'Terima barang', sub: 'Catat barang yang datang dari penitip atau pembelian toko; stok otomatis bertambah', add: 'Terima barang', one: 'Penerimaan', many: 'Penerimaan', flow: 'Barang masuk', empty: 'Tidak ada penerimaan pada saringan ini. Pilih Terima barang saat ada kiriman.', sign: '+' },
  )

  const monthStart = new Date()
  monthStart.setDate(1)

  let receipts = $state<Receipt[]>([])
  let members = $state<Member[]>([])
  let search = $state('')
  let memberId = $state(0)
  let from = $state(isoDate(monthStart))
  let to = $state(isoDate())
  const param = $derived(routeParam())

  let detail = $state<Receipt | null>(null)
  let confirming = $state(false)
  let busy = $state(false)
  let error = $state('')
  let pending = $state<DocDraft | null>(null)

  const valid = $derived(receipts.filter((r) => !r.cancelled_at))
  const totalQty = $derived(valid.reduce((sum, r) => sum + r.total_qty, 0))
  const ownerCount = $derived(new Set(valid.map((r) => r.member_id)).size)
  const pendingQty = $derived(Object.values(pending?.qty ?? {}).reduce((sum, n) => sum + n, 0))
  const pendingOwner = $derived(members.find((m) => m.id === pending?.memberId)?.name ?? 'Barang toko')
  const oversold = $derived((detail?.items ?? []).filter((i) => i.stock < i.qty))

  let requestNo = 0
  async function load() {
    const current = ++requestNo
    const params = new URLSearchParams({ q: search.trim(), member_id: String(memberId), from, to })
    const res = await api<Receipt[]>(`/api/${kind}?${params}`)
    if (current === requestNo) receipts = res
  }

  let timer = 0
  function searchChanged() {
    clearTimeout(timer)
    timer = setTimeout(load, 250)
  }

  async function open(id: number) {
    confirming = false
    error = ''
    detail = await api<Receipt>(`/api/${kind}/${id}`)
  }

  async function run(action: () => Promise<void>) {
    busy = true
    error = ''
    try {
      await action()
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  const cancel = (refill: boolean) =>
    run(async () => {
      const doc = detail!
      await post(`/api/receipts/${doc.id}/cancel`, {})
      confirming = false
      if (refill) {
        const qty = Object.fromEntries((doc.items ?? []).map((i) => [i.product_id, i.qty]))
        writeDraft(kind, { memberId: doc.member_id, note: doc.note, qty })
        detail = null
        location.hash = '#/receipts/new'
        return
      }
      await open(doc.id)
      await load()
    })

  $effect(() => {
    void param
    pending = readDraft(kind)
    load()
  })

  listMembers().then((m) => (members = m))
</script>

<div class="page-head">
  <div class="titles">
    <h1>{text.title}</h1>
    <p>{text.sub}</p>
  </div>
  <a class="button primary" href="#/{kind}/new"><Icon name="plus" /> {text.add}</a>
</div>

{#if pending && param !== 'new'}
  <p class="pending">
    <span>Ada isian yang belum disimpan: <strong>{pendingOwner}, {rupiah(pendingQty)} pcs</strong></span>
    <a class="button" href="#/{kind}/new">Lanjutkan</a>
  </p>
{/if}

<div class="filters">
  <div class="search">
    <Icon name="search" />
    <input placeholder="Cari no., catatan, nama barang, atau scan barcode" bind:value={search} oninput={searchChanged} />
  </div>
  <select bind:value={memberId} onchange={load} aria-label="Pemilik">
    <option value={0}>Semua pemilik</option>
    {#if !isReturn}<option value={-1}>Barang toko</option>{/if}
    {#each members as m (m.id)}
      <option value={m.id}>{m.name}</option>
    {/each}
  </select>
  <input type="date" bind:value={from} onchange={load} aria-label="Dari tanggal" />
  <input type="date" bind:value={to} onchange={load} aria-label="Sampai tanggal" />
</div>

<div class="stats">
  <div class="key">{text.flow}<b>{rupiah(totalQty)} pcs</b></div>
  <div>{text.many}<b>{valid.length}</b></div>
  <div>Pemilik<b>{ownerCount}</b></div>
</div>

{#if receipts.length >= DOC_LIMIT}
  <p class="muted">Hanya {DOC_LIMIT} dokumen terbaru yang ditampilkan dan dihitung. Persempit tanggal atau pemiliknya.</p>
{/if}

<div class="table-card">
  <table>
    <thead>
      <tr>
        <th>No.</th>
        <th>Tanggal</th>
        <th>Pemilik</th>
        <th>Catatan</th>
        <th class="num">Produk</th>
        <th class="num">Jumlah</th>
        <th>Petugas</th>
        {#if !isReturn}<th></th>{/if}
      </tr>
    </thead>
    <tbody>
      {#each receipts as r (r.id)}
        <tr class="clickable" class:cancelled={r.cancelled_at} onclick={() => open(r.id)}>
          <td class="code">#{r.id}</td>
          <td>{dateTime(r.received_at)}</td>
          <td class="strong">
            {#if r.member_id}{r.member_name}{:else}<span class="badge store">Toko</span>{/if}
            {#if r.cancelled_at}<span class="badge off">Dibatalkan</span>{/if}
          </td>
          <td>{r.note || '—'}</td>
          <td class="num">{r.item_count}</td>
          <td class="num">{rupiah(r.total_qty)}</td>
          <td>{r.received_by}</td>
          {#if !isReturn}
            <td class="num">
              {#if !r.cancelled_at}<a class="row-link" href="#/labels/receipt/{r.id}" onclick={(e) => e.stopPropagation()}>Label</a>{/if}
            </td>
          {/if}
        </tr>
      {:else}
        <tr><td colspan="8" class="empty">{text.empty}</td></tr>
      {/each}
    </tbody>
  </table>
</div>

{#if detail}
  <Drawer title="{text.one} #{detail.id}" onclose={() => (detail = null)}>
    <div class="drawer-body">
      <p class="meta">
        {detail.member_id ? detail.member_name : 'Barang toko'} · {dateTime(detail.received_at)} · {detail.received_by}
        {#if detail.note}<br />{detail.note}{/if}
      </p>
      {#if detail.cancelled_at}
        <p class="gone">Dibatalkan {dateTime(detail.cancelled_at)} oleh {detail.cancelled_by}. Stoknya sudah ditarik kembali.</p>
      {/if}
      <p class="section">{detail.item_count} produk · {rupiah(detail.total_qty)} pcs</p>
      {#each detail.items ?? [] as item (item.product_id)}
        <div class="item">
          <div>
            <strong>{item.name}</strong>
            <small>{item.barcode || 'tanpa barcode'} · stok sekarang {item.stock}</small>
          </div>
          <span class:neg={isReturn}>{text.sign}{rupiah(item.qty)}</span>
        </div>
      {/each}
    </div>
    {#if error}<p class="error foot-error">{error}</p>{/if}
    {#if !isReturn && !detail.cancelled_at}
      {@const doc = detail}
      {#if confirming}
        <div class="confirm">
          <p>Stok {doc.item_count} barang dikurangi {rupiah(doc.total_qty)} pcs, dan penerimaan ini tidak dihitung lagi.</p>
          {#if oversold.length > 0}
            <p class="neg">
              {oversold.length} barang jadi minus karena sudah terjual:
              {oversold.map((i) => `${i.name} (${i.stock - i.qty})`).join(', ')}
            </p>
          {/if}
          <div class="confirm-actions">
            <button disabled={busy} onclick={() => (confirming = false)}>Tidak jadi</button>
            <button disabled={busy} onclick={() => cancel(false)}>Batalkan saja</button>
            {#if !pending}<button class="primary" disabled={busy} onclick={() => cancel(true)}>Batalkan & isi ulang</button>{/if}
          </div>
        </div>
      {:else}
        <div class="drawer-foot">
          <button class="link undo" onclick={() => (confirming = true)}>Salah input?</button>
          <button disabled={busy} onclick={() => run(() => printGoodsReceipt(doc.id))}><Icon name="printer" /> Tanda terima</button>
          <a class="button primary" href="#/labels/receipt/{doc.id}"><Icon name="tag" /> Cetak label</a>
        </div>
      {/if}
    {/if}
  </Drawer>
{/if}
{#if param === 'new'}
  <ReceiptForm {kind} />
{/if}

<style>
  .meta {
    margin: 0;
    color: var(--muted);
  }
  .item {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .item div {
    display: grid;
  }
  .item strong {
    font-weight: 500;
  }
  .item span,
  small {
    font-family: var(--mono);
  }
  small {
    font-size: 0.8rem;
    color: var(--muted);
  }
  .item span {
    color: var(--accent);
  }
  .filters input[type='date'] {
    width: auto;
  }
  .pending {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    margin: 0 0 1rem;
    padding: 0.6rem 1rem;
    border: 1px solid var(--line-strong);
    border-left: 3px solid var(--amber);
    border-radius: 8px;
    background: var(--card);
  }
  .pending strong {
    font-weight: 500;
  }
  p.muted {
    margin: 0 0 0.9rem;
  }
  tr.cancelled td {
    color: var(--muted);
  }
  .gone {
    margin: 0;
    padding: 0.55rem 0.75rem;
    border-radius: 7px;
    background: var(--danger-wash);
    color: var(--danger);
  }
  .foot-error {
    margin: 0;
    padding: 0.5rem 1.4rem;
  }
  .undo {
    margin-right: auto;
    color: var(--danger);
  }
  .confirm {
    display: grid;
    gap: 0.6rem;
    padding: 0.9rem 1.4rem;
    border-top: 1px solid var(--line);
    background: var(--danger-wash);
  }
  .confirm p {
    margin: 0;
  }
  .confirm-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.5rem;
  }
  @media (max-width: 860px) {
    .filters {
      flex-wrap: wrap;
    }
  }
</style>
