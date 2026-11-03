<script lang="ts">
  import Drawer from '../../components/Drawer.svelte'
  import Icon from '../../components/Icon.svelte'
  import { api, post } from '../../lib/api'
  import { isoDate, rupiah } from '../../lib/format'
  import { printRecap, printReceipt } from '../../lib/receipt'
  import { describeMethods, methodLabels, type Recap, type Transaction } from '../../lib/sales'
  import { session } from '../../lib/session.svelte'

  const time = new Intl.DateTimeFormat('id-ID', { timeStyle: 'short' })
  const statusLabels = { paid: 'Lunas', pending: 'Menunggu', void: 'Batal' }
  const paymentStatus = { paid: 'lunas', pending: 'menunggu', failed: 'gagal', expired: 'kedaluwarsa' }

  let date = $state(isoDate())
  let transactions = $state<Transaction[]>([])
  let detail = $state<Transaction | null>(null)
  let message = $state('')
  let failed = $state(false)

  let recap = $state<Recap | null>(null)

  async function load() {
    ;[transactions, recap] = await Promise.all([
      api<Transaction[]>(`/api/transactions?date=${date}`),
      api<Recap>(`/api/recap?date=${date}`),
    ])
  }

  async function open(id: number) {
    message = ''
    detail = await api<Transaction>(`/api/transactions/${id}`)
  }

  let recapNote = $state('')

  async function printDay() {
    if (!recap) return
    try {
      await printRecap(recap)
      recapNote = ''
    } catch (err) {
      recapNote = `Rekap tidak tercetak: ${(err as Error).message}`
    }
  }

  async function reprint() {
    if (!detail) return
    try {
      await printReceipt(detail, false)
      message = 'Struk dikirim ke printer.'
      failed = false
    } catch (err) {
      message = (err as Error).message
      failed = true
    }
  }

  async function voidSale() {
    if (!detail || !confirm(`Batalkan transaksi ${detail.invoice_no}? Stok dikembalikan; uang dikembalikan manual.`)) return
    try {
      await post(`/api/transactions/${detail.id}/void`)
      await Promise.all([open(detail.id), load()])
      message = 'Transaksi dibatalkan.'
      failed = false
    } catch (err) {
      message = (err as Error).message
      failed = true
    }
  }

  $effect(() => {
    date
    load()
  })
</script>

<div class="page-head">
  <div class="titles">
    <h1>Transaksi</h1>
    <p>Rekap dan daftar transaksi per hari</p>
  </div>
  <label class="date">Tanggal <input type="date" bind:value={date} max={isoDate()} /></label>
  <button disabled={!recap} onclick={printDay}><Icon name="printer" /> Cetak rekap</button>
</div>

{#if recap}
  <div class="stats">
    <div>Transaksi lunas<b>{recap.transactions}</b></div>
    <div>Tunai<b>Rp {rupiah(recap.cash)}</b></div>
    <div>Debit<b>Rp {rupiah(recap.debit)}</b></div>
    <div>QRIS<b>Rp {rupiah(recap.qris)}</b></div>
    <div class="key">Total<b>Rp {rupiah(recap.total)}</b></div>
  </div>
{/if}
{#if recapNote}<p class="muted">{recapNote}</p>{/if}

<div class="table-card">
  <table>
    <thead>
      <tr>
        <th>No. struk</th>
        <th>Jam</th>
        <th>Kasir</th>
        <th>Pembayaran</th>
        <th class="num">Total</th>
        <th>Status</th>
      </tr>
    </thead>
    <tbody>
      {#each transactions as t (t.id)}
        <tr class="clickable" class:inactive={t.status === 'void'} onclick={() => open(t.id)}>
          <td class="code">{t.invoice_no}</td>
          <td>{time.format(new Date(t.created_at))}</td>
          <td>{t.cashier}</td>
          <td>{describeMethods(t.methods)}</td>
          <td class="num">{rupiah(t.total)}</td>
          <td>
            <span class="badge" class:off={t.status === 'void'} class:draft={t.status === 'pending'}>
              {statusLabels[t.status]}
            </span>
          </td>
        </tr>
      {:else}
        <tr><td colspan="6" class="empty">Belum ada transaksi pada tanggal ini.</td></tr>
      {/each}
    </tbody>
  </table>
</div>

{#if detail}
  <Drawer title={detail.invoice_no} onclose={() => (detail = null)}>
    <div class="drawer-body">
      <p class="meta">
        {new Date(detail.created_at).toLocaleString('id-ID')} · {detail.cashier} ·
        <strong>{statusLabels[detail.status]}</strong>
      </p>
      <p class="section">Barang</p>
      {#each detail.items ?? [] as item}
        <div class="line">
          <div>
            <strong>{item.name}</strong>
            <small>{item.qty} × {rupiah(item.unit_price)}</small>
          </div>
          <span>{rupiah(item.line_total)}</span>
        </div>
      {/each}
      <div class="line total"><strong>Total</strong><span>{rupiah(detail.total)}</span></div>
      <p class="section">Pembayaran</p>
      {#each detail.payments ?? [] as p}
        <div class="line">
          <div>
            <strong>{methodLabels[p.method]}{p.status === 'paid' ? '' : ` (${paymentStatus[p.status]})`}</strong>
            {#if p.reference}<small>Ref {p.reference}</small>{/if}
            {#if p.tendered !== null && p.tendered > p.amount}
              <small>Diterima {rupiah(p.tendered)}, kembali {rupiah(p.tendered - p.amount)}</small>
            {/if}
          </div>
          <span>{rupiah(p.amount)}</span>
        </div>
      {/each}
      {#if message}<p class:error={failed} class="note">{message}</p>{/if}
    </div>
    <div class="drawer-foot">
      <button class="primary" onclick={reprint}><Icon name="printer" /> Cetak ulang</button>
      {#if detail.status === 'pending' || (session.user?.role === 'admin' && detail.status !== 'void')}
        <button onclick={voidSale}>Batalkan</button>
      {/if}
    </div>
  </Drawer>
{/if}

<style>
  .date {
    grid-auto-flow: column;
    align-items: center;
    gap: 0.6rem;
  }
  .meta,
  .note {
    margin: 0;
    color: var(--muted);
  }
  .line {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 1rem;
  }
  .line div {
    display: grid;
  }
  .line strong {
    font-weight: 500;
  }
  .line span,
  small {
    font-family: var(--mono);
  }
  small {
    font-size: 0.8rem;
    color: var(--muted);
  }
  .total {
    padding-top: 0.7rem;
    border-top: 1px solid var(--line-strong);
    font-size: 1.1rem;
  }
</style>
