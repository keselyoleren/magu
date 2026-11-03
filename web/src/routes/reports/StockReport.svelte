<script lang="ts">
  import Icon from '../../components/Icon.svelte'
  import { api, post } from '../../lib/api'
  import { isoDate, rupiah } from '../../lib/format'
  import { driveFolderUrl, listMembers, type Member } from '../../lib/members'
  import { loadSettings } from '../../lib/sales'

  interface Row {
    product_id: number
    barcode: string
    name: string
    price: number
    stock: number
    received: number
    sold: number
    returned: number
    adjusted: number
  }

  const now = new Date()
  let members = $state<Member[]>([])
  let memberId = $state(0)
  let from = $state(isoDate(new Date(now.getFullYear(), now.getMonth(), 1)))
  let to = $state(isoDate())
  let rows = $state<Row[]>([])
  let driveEnabled = $state(false)
  let sending = $state(false)
  let error = $state('')
  let sentFolder = $state('')

  const owner = $derived(members.find((m) => m.id === memberId))
  const totals = $derived(
    rows.reduce(
      (t, r) => ({
        received: t.received + r.received,
        sold: t.sold + r.sold,
        returned: t.returned + r.returned,
        adjusted: t.adjusted + r.adjusted,
        stock: t.stock + r.stock,
        value: t.value + Math.max(r.stock, 0) * r.price,
      }),
      { received: 0, sold: 0, returned: 0, adjusted: 0, stock: 0, value: 0 },
    ),
  )

  $effect(() => {
    api<Row[]>(`/api/reports/stock?member_id=${memberId}&from=${from}&to=${to}`).then((r) => (rows = r))
  })

  listMembers().then((m) => {
    members = m
    memberId = m.find((x) => x.is_active)?.id ?? 0
  })

  loadSettings().then((s) => (driveEnabled = s.drive_enabled))

  async function sendToDrive() {
    sending = true
    error = ''
    sentFolder = ''
    try {
      const sent = await post<{ folder_id: string }>(`/api/reports/stock/drive?member_id=${memberId}&from=${from}&to=${to}`)
      sentFolder = sent.folder_id
    } catch (err) {
      error = (err as Error).message
    } finally {
      sending = false
    }
  }
</script>

<div class="page-head">
  <div class="titles">
    <h1>Stok titipan</h1>
    <p>
      {owner ? `${owner.name} (${owner.code})` : 'Barang toko'} · mutasi {from} s.d. {to}, sisa stok per hari ini
    </p>
  </div>
  <a class="button no-print" href="/api/reports/stock/xlsx?member_id={memberId}&from={from}&to={to}" download><Icon name="download" /> Excel</a>
  {#if driveEnabled && memberId > 0}
    <button class="no-print" disabled={sending} onclick={sendToDrive}><Icon name="upload" /> Kirim ke Drive</button>
  {/if}
  <button class="no-print" onclick={() => window.print()}><Icon name="printer" /> Cetak</button>
</div>

{#if error}<p class="error no-print">{error}</p>{/if}
{#if sentFolder}
  <p class="no-print">
    Laporan terkirim. <a href={driveFolderUrl(sentFolder)} target="_blank" rel="noopener">Buka folder member</a>
  </p>
{/if}

<div class="filters no-print">
  <select bind:value={memberId}>
    {#each members as m (m.id)}
      <option value={m.id}>{m.code} — {m.name}</option>
    {/each}
    <option value={0}>Barang toko</option>
  </select>
  <input type="date" bind:value={from} max={to} aria-label="Dari tanggal" />
  <input type="date" bind:value={to} min={from} max={isoDate()} aria-label="Sampai tanggal" />
</div>

<div class="stats">
  <div>Masuk<b>{rupiah(totals.received)}</b></div>
  <div>Terjual<b>{rupiah(totals.sold)}</b></div>
  <div>Diretur<b>{rupiah(totals.returned)}</b></div>
  <div class="key">Sisa di toko<b>{rupiah(totals.stock)}</b></div>
  <div>Nilai sisa<b>Rp {rupiah(totals.value)}</b></div>
</div>

<div class="table-card">
  <table>
    <thead>
      <tr>
        <th>Barcode</th>
        <th>Produk</th>
        <th class="num">Harga</th>
        <th class="num">Masuk</th>
        <th class="num">Terjual</th>
        <th class="num">Retur</th>
        <th class="num">Selisih opname</th>
        <th class="num">Sisa stok</th>
      </tr>
    </thead>
    <tbody>
      {#each rows as r (r.product_id)}
        <tr>
          <td class="code">{r.barcode || '—'}</td>
          <td class="strong">{r.name}</td>
          <td class="num">{rupiah(r.price)}</td>
          <td class="num">{r.received}</td>
          <td class="num">{r.sold}</td>
          <td class="num">{r.returned}</td>
          <td class="num" class:neg={r.adjusted < 0} class:pos={r.adjusted > 0}>{r.adjusted > 0 ? `+${r.adjusted}` : r.adjusted}</td>
          <td class="num" class:neg={r.stock < 0}>{r.stock}</td>
        </tr>
      {:else}
        <tr><td colspan="8" class="empty">Pemilik ini belum punya produk.</td></tr>
      {/each}
    </tbody>
    <tfoot>
      <tr>
        <td colspan="3">Total</td>
        <td class="num">{totals.received}</td>
        <td class="num">{totals.sold}</td>
        <td class="num">{totals.returned}</td>
        <td class="num">{totals.adjusted}</td>
        <td class="num">{totals.stock}</td>
      </tr>
    </tfoot>
  </table>
</div>

<style>
  .filters input {
    max-width: 170px;
  }
</style>
