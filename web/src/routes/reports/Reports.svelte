<script lang="ts">
  import Icon from '../../components/Icon.svelte'
  import { api } from '../../lib/api'
  import { dateOnly, isoDate, rupiah } from '../../lib/format'

  interface Report {
    daily: {
      day: string
      transactions: number
      gross: number
      store_share: number
      vendor_share: number
      cash: number
      debit: number
      qris: number
    }[]
    owners: { member_id: number | null; name: string; qty: number; gross: number; store_share: number; vendor_share: number }[]
  }

  const now = new Date()
  let from = $state(isoDate(new Date(now.getFullYear(), now.getMonth(), 1)))
  let to = $state(isoDate())
  let report = $state<Report>({ daily: [], owners: [] })
  let error = $state('')

  const totals = $derived(
    report.daily.reduce(
      (t, d) => ({
        transactions: t.transactions + d.transactions,
        gross: t.gross + d.gross,
        store: t.store + d.store_share,
        vendor: t.vendor + d.vendor_share,
      }),
      { transactions: 0, gross: 0, store: 0, vendor: 0 },
    ),
  )
  const busiest = $derived(Math.max(1, ...report.daily.map((d) => d.gross)))

  $effect(() => {
    api<Report>(`/api/reports/sales?from=${from}&to=${to}`)
      .then((r) => {
        report = r
        error = ''
      })
      .catch((err) => (error = (err as Error).message))
  })

</script>

<div class="page-head">
  <div class="titles">
    <h1>Laporan penjualan</h1>
    <p>Penjualan lunas, dibagi menjadi hak toko dan hak penitip</p>
  </div>
  <label class="date no-print">Dari <input type="date" bind:value={from} max={to} /></label>
  <label class="date no-print">Sampai <input type="date" bind:value={to} min={from} max={isoDate()} /></label>
  <a class="button no-print" href="/api/reports/sales/xlsx?from={from}&to={to}" download><Icon name="download" /> Excel</a>
</div>

{#if error}<p class="error">{error}</p>{/if}

<div class="stats">
  <div>Penjualan<b>Rp {rupiah(totals.gross)}</b></div>
  <div class="key">Hak toko<b>Rp {rupiah(totals.store)}</b></div>
  <div>Hak penitip<b>Rp {rupiah(totals.vendor)}</b></div>
  <div>Transaksi<b>{rupiah(totals.transactions)}</b></div>
</div>

<h2 class="sub">Per hari</h2>
<div class="table-card">
  <table>
    <thead>
      <tr>
        <th>Tanggal</th>
        <th class="bar-col"></th>
        <th class="num">Transaksi</th>
        <th class="num">Tunai</th>
        <th class="num">Debit</th>
        <th class="num">QRIS</th>
        <th class="num">Penjualan</th>
        <th class="num">Hak toko</th>
        <th class="num">Hak penitip</th>
      </tr>
    </thead>
    <tbody>
      {#each report.daily as d (d.day)}
        <tr>
          <td>{dateOnly(d.day)}</td>
          <td class="bar-col"><span class="bar" style="width: {(d.gross / busiest) * 100}%"></span></td>
          <td class="num">{d.transactions}</td>
          <td class="num">{rupiah(d.cash)}</td>
          <td class="num">{rupiah(d.debit)}</td>
          <td class="num">{rupiah(d.qris)}</td>
          <td class="num">{rupiah(d.gross)}</td>
          <td class="num">{rupiah(d.store_share)}</td>
          <td class="num">{rupiah(d.vendor_share)}</td>
        </tr>
      {:else}
        <tr><td colspan="9" class="empty">Tidak ada penjualan pada rentang ini.</td></tr>
      {/each}
    </tbody>
  </table>
</div>

<h2 class="sub">Per pemilik barang</h2>
<div class="table-card">
  <table>
    <thead>
      <tr>
        <th>Pemilik</th>
        <th class="num">Jumlah barang</th>
        <th class="num">Penjualan</th>
        <th class="num">Hak toko</th>
        <th class="num">Hak penitip</th>
      </tr>
    </thead>
    <tbody>
      {#each report.owners as o (o.member_id)}
        <tr>
          <td class="strong">
            {#if o.member_id}{o.name}{:else}<span class="badge store">Toko</span>{/if}
          </td>
          <td class="num">{rupiah(o.qty)}</td>
          <td class="num">{rupiah(o.gross)}</td>
          <td class="num">{rupiah(o.store_share)}</td>
          <td class="num">{rupiah(o.vendor_share)}</td>
        </tr>
      {:else}
        <tr><td colspan="5" class="empty">Tidak ada penjualan pada rentang ini.</td></tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .date {
    grid-auto-flow: column;
    align-items: center;
    gap: 0.5rem;
  }
  .bar-col {
    width: 14%;
  }
  .bar {
    display: block;
    height: 8px;
    min-width: 2px;
    border-radius: 4px;
    background: var(--accent);
    opacity: 0.75;
  }
</style>
