<script lang="ts">
  import Modal from '../../components/Modal.svelte'
  import { api, post } from '../../lib/api'
  import { dateOnly, isoDate, rupiah } from '../../lib/format'
  import type { Outstanding } from './Settlements.svelte'

  let { memberId: initialMember }: { memberId: number } = $props()

  let outstanding = $state<Outstanding[]>([])
  // svelte-ignore state_referenced_locally
  let memberId = $state(initialMember || 0)
  let until = $state(isoDate())
  let error = $state('')
  let busy = $state(false)

  const chosen = $derived(outstanding.find((o) => o.member_id === memberId))

  async function save(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      const { id } = await post<{ id: number }>('/api/settlements', { member_id: memberId, until })
      location.hash = `#/settlements/${id}`
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  api<Outstanding[]>('/api/settlements/outstanding').then((o) => (outstanding = o))
</script>

<Modal title="Buat pembagian hasil" onclose={() => (location.hash = '#/settlements')}>
  <form class="modal-form" onsubmit={save}>
    <div class="form-body">
        <p class="modal-hint">Mengumpulkan penjualan lunas seorang penitip menjadi satu tagihan yang dibayarkan kepadanya</p>
      <div class="pair">
        <label>
          Member
          <select bind:value={memberId} required>
            <option value={0} disabled>Pilih member</option>
            {#each outstanding as o (o.member_id)}
              <option value={o.member_id}>{o.member_code} — {o.member_name}</option>
            {/each}
          </select>
        </label>
        <label>Penjualan sampai tanggal <input type="date" bind:value={until} max={isoDate()} required /></label>
      </div>
      {#if chosen}
        <p class="muted">
          Belum dibagi sejak {dateOnly(chosen.since)}: penjualan Rp {rupiah(chosen.gross_sales)}, hak toko Rp
          {rupiah(chosen.commission_total)}, hak penitip <strong>Rp {rupiah(chosen.payout_amount)}</strong>. Angka akhirnya
          mengikuti tanggal batas di atas.
        </p>
      {/if}
      {#if error}<p class="error">{error}</p>{/if}
    </div>
    <div class="form-actions">
      <a class="button" href="#/settlements">Batal</a>
      <button class="primary" disabled={busy || !memberId}>Buat pembagian</button>
    </div>
  </form>
</Modal>

<style>
  p.muted {
    margin: 0;
  }
  strong {
    color: var(--text);
  }
</style>
