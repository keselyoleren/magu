<script lang="ts" module>
  export interface Settlement {
    id: number
    member_id: number
    member_code: string
    member_name: string
    bank: string
    period_start: string
    period_end: string
    gross_sales: number
    commission_total: number
    payout_amount: number
    status: 'draft' | 'paid'
    created_at: string
    paid_at: string | null
    drive_sent_at: string | null
    lines?: {
      paid_at: string
      invoice_no: string
      product_name: string
      qty: number
      line_total: number
      commission_bps: number
      commission_amount: number
      vendor_amount: number
    }[]
  }

  export interface Outstanding {
    member_id: number
    member_code: string
    member_name: string
    gross_sales: number
    commission_total: number
    payout_amount: number
    since: string
  }
</script>

<script lang="ts">
  import { api } from '../../lib/api'
  import { dateOnly, rupiah } from '../../lib/format'
  import { routeParam } from '../../lib/route.svelte'
  import SettlementDetail from './SettlementDetail.svelte'
  import SettlementForm from './SettlementForm.svelte'

  const param = $derived(routeParam())

  let outstanding = $state<Outstanding[]>([])
  let settlements = $state<Settlement[]>([])

  const owed = $derived(outstanding.reduce((sum, o) => sum + o.payout_amount, 0))
  const unpaid = $derived(settlements.filter((s) => s.status === 'draft').reduce((sum, s) => sum + s.payout_amount, 0))

  $effect(() => {
    if (param && param !== 'new') return
    api<Outstanding[]>('/api/settlements/outstanding').then((o) => (outstanding = o))
    api<Settlement[]>('/api/settlements').then((s) => (settlements = s))
  })
</script>

{#if param && param !== 'new'}
  {#key param}
    <SettlementDetail id={Number(param)} />
  {/key}
{:else}
  <div class="page-head">
    <div class="titles">
      <h1>Bagi hasil</h1>
      <p>Hak penitip dari penjualan yang sudah lunas</p>
    </div>
  </div>

  <div class="stats">
    <div class="key">Belum dibagi<b>Rp {rupiah(owed)}</b></div>
    <div>Sudah dibagi, belum dibayar<b>Rp {rupiah(unpaid)}</b></div>
    <div>Penitip menunggu<b>{outstanding.length}</b></div>
  </div>

  <h2 class="sub">Belum dibagi</h2>
  <div class="table-card">
    <table>
      <thead>
        <tr>
          <th>Member</th>
          <th>Penjualan sejak</th>
          <th class="num">Penjualan</th>
          <th class="num">Hak toko</th>
          <th class="num">Hak penitip</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {#each outstanding as o (o.member_id)}
          <tr>
            <td class="strong">{o.member_name} <span class="muted">{o.member_code}</span></td>
            <td>{dateOnly(o.since)}</td>
            <td class="num">{rupiah(o.gross_sales)}</td>
            <td class="num">{rupiah(o.commission_total)}</td>
            <td class="num">{rupiah(o.payout_amount)}</td>
            <td class="num"><a class="row-link" href="#/settlements/new/{o.member_id}">Buat pembagian</a></td>
          </tr>
        {:else}
          <tr><td colspan="6" class="empty">Semua penjualan sudah dibagi.</td></tr>
        {/each}
      </tbody>
    </table>
  </div>

  <h2 class="sub">Riwayat pembagian</h2>
  <div class="table-card">
    <table>
      <thead>
        <tr>
          <th>No.</th>
          <th>Member</th>
          <th>Periode</th>
          <th class="num">Penjualan</th>
          <th class="num">Hak toko</th>
          <th class="num">Dibayar ke penitip</th>
          <th>Status</th>
        </tr>
      </thead>
      <tbody>
        {#each settlements as s (s.id)}
          <tr class="clickable" onclick={() => (location.hash = `#/settlements/${s.id}`)}>
            <td class="code">#{s.id}</td>
            <td class="strong">{s.member_name}</td>
            <td>{dateOnly(s.period_start)} – {dateOnly(s.period_end)}</td>
            <td class="num">{rupiah(s.gross_sales)}</td>
            <td class="num">{rupiah(s.commission_total)}</td>
            <td class="num">{rupiah(s.payout_amount)}</td>
            <td><span class="badge" class:draft={s.status === 'draft'}>{s.status === 'draft' ? 'Belum dibayar' : 'Dibayar'}</span></td>
          </tr>
        {:else}
          <tr><td colspan="7" class="empty">Belum ada pembagian hasil.</td></tr>
        {/each}
      </tbody>
    </table>
  </div>
  {#if param === 'new'}
    <SettlementForm memberId={Number(routeParam(1))} />
  {/if}
{/if}
