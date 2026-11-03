<script lang="ts">
  import Icon from '../../components/Icon.svelte'
  import { api, del, post } from '../../lib/api'
  import { bpsToPercent, dateOnly, dateTime, rupiah } from '../../lib/format'
  import { loadSettings } from '../../lib/sales'
  import type { Settlement } from './Settlements.svelte'

  let { id }: { id: number } = $props()

  let settlement = $state<Settlement | null>(null)
  let error = $state('')
  let busy = $state(false)
  let driveEnabled = $state(false)
  let sending = $state(false)

  async function load() {
    try {
      settlement = await api<Settlement>(`/api/settlements/${id}`)
    } catch (err) {
      error = (err as Error).message
    }
  }

  async function markPaid() {
    if (!settlement || !confirm(`Tandai Rp ${rupiah(settlement.payout_amount)} sudah dibayarkan ke ${settlement.member_name}?`)) return
    busy = true
    try {
      await post(`/api/settlements/${id}/pay`)
      await load()
      if ((await loadSettings()).drive_enabled) await sendToDrive()
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  async function sendToDrive() {
    sending = true
    error = ''
    try {
      await post(`/api/settlements/${id}/drive`)
      await load()
    } catch (err) {
      error = `Laporan belum terkirim ke Drive: ${(err as Error).message}`
    } finally {
      sending = false
    }
  }

  async function remove() {
    if (!confirm('Hapus draft ini? Penjualannya kembali ke daftar "belum dibagi".')) return
    try {
      await del(`/api/settlements/${id}`)
      location.hash = '#/settlements'
    } catch (err) {
      error = (err as Error).message
    }
  }

  load()
  loadSettings().then((s) => (driveEnabled = s.drive_enabled))
</script>

<a class="back" href="#/settlements"><Icon name="back" size={16} /> Bagi hasil</a>

{#if settlement}
  <div class="page-head">
    <div class="titles">
      <h1>Pembagian hasil #{settlement.id}</h1>
      <p>
        {settlement.member_name} ({settlement.member_code}) · {dateOnly(settlement.period_start)} –
        {dateOnly(settlement.period_end)}
        {#if settlement.paid_at}· dibayar {dateTime(settlement.paid_at)}{/if}
        {#if settlement.drive_sent_at}· terkirim ke Drive {dateTime(settlement.drive_sent_at)}{/if}
      </p>
    </div>
    <a class="button no-print" href="/api/settlements/{id}/xlsx" download><Icon name="download" /> Excel</a>
    <button class="no-print" onclick={() => window.print()}><Icon name="printer" /> Cetak</button>
    {#if settlement.status === 'draft'}
      <button class="no-print" onclick={remove}><Icon name="trash" /> Hapus draft</button>
      <button class="primary no-print" disabled={busy} onclick={markPaid}>Tandai dibayar</button>
    {:else}
      {#if driveEnabled}
        <button class="no-print" disabled={sending} onclick={sendToDrive}>
          <Icon name="upload" /> {settlement.drive_sent_at ? 'Kirim ulang ke Drive' : 'Kirim ke Drive'}
        </button>
      {/if}
      <span class="badge">Dibayar</span>
    {/if}
  </div>

  {#if error}<p class="error">{error}</p>{/if}

  <div class="stats">
    <div>Penjualan<b>Rp {rupiah(settlement.gross_sales)}</b></div>
    <div>Hak toko<b>Rp {rupiah(settlement.commission_total)}</b></div>
    <div class="key">Dibayar ke penitip<b>Rp {rupiah(settlement.payout_amount)}</b></div>
    <div>Rekening<b class="bank">{settlement.bank || 'Belum diisi'}</b></div>
  </div>

  <div class="table-card">
    <table>
      <thead>
        <tr>
          <th>Tanggal</th>
          <th>No. struk</th>
          <th>Produk</th>
          <th class="num">Jumlah</th>
          <th class="num">Penjualan</th>
          <th class="num">Komisi</th>
          <th class="num">Hak toko</th>
          <th class="num">Hak penitip</th>
        </tr>
      </thead>
      <tbody>
        {#each settlement.lines ?? [] as l}
          <tr>
            <td>{dateTime(l.paid_at)}</td>
            <td class="code">{l.invoice_no}</td>
            <td class="strong">{l.product_name}</td>
            <td class="num">{l.qty}</td>
            <td class="num">{rupiah(l.line_total)}</td>
            <td class="num">{bpsToPercent(l.commission_bps)}%</td>
            <td class="num">{rupiah(l.commission_amount)}</td>
            <td class="num">{rupiah(l.vendor_amount)}</td>
          </tr>
        {/each}
      </tbody>
      <tfoot>
        <tr>
          <td colspan="4">Total</td>
          <td class="num">{rupiah(settlement.gross_sales)}</td>
          <td></td>
          <td class="num">{rupiah(settlement.commission_total)}</td>
          <td class="num">{rupiah(settlement.payout_amount)}</td>
        </tr>
      </tfoot>
    </table>
  </div>
{:else if error}
  <p class="error">{error}</p>
{/if}

<style>
  .bank {
    font-family: var(--sans);
    font-size: 1rem;
  }
</style>
