<script lang="ts" module>
  export interface Opname {
    id: number
    member_id: number | null
    member_name: string
    status: 'draft' | 'posted'
    note: string
    created_at: string
    posted_at: string | null
    item_count: number
    counted: number
    diff_count: number
    items?: { product_id: number; barcode: string; name: string; system_qty: number; physical_qty: number | null }[]
  }
</script>

<script lang="ts">
  import Icon from '../../components/Icon.svelte'
  import Modal from '../../components/Modal.svelte'
  import { api, post } from '../../lib/api'
  import { dateTime } from '../../lib/format'
  import { listMembers, type Member } from '../../lib/members'
  import { routeParam } from '../../lib/route.svelte'
  import OpnameDetail from './OpnameDetail.svelte'

  const param = $derived(routeParam())

  let opnames = $state<Opname[]>([])
  let members = $state<Member[]>([])
  let memberId = $state<number | null>(null)
  let note = $state('')
  let error = $state('')
  let busy = $state(false)

  $effect(() => {
    if (!param || param === 'new') api<Opname[]>('/api/opnames').then((o) => (opnames = o))
  })

  async function create(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      const { id } = await post<{ id: number }>('/api/opnames', { member_id: memberId, note })
      note = ''
      location.hash = `#/opnames/${id}`
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  listMembers().then((m) => (members = m))
</script>

{#if param && param !== 'new'}
  {#key param}
    <OpnameDetail id={Number(param)} />
  {/key}
{:else}
  <div class="page-head">
    <div class="titles">
      <h1>Stok opname</h1>
      <p>Hitung semua barang satu pemilik sekaligus. Untuk membetulkan beberapa barang saja, pakai Atur stok</p>
    </div>
    <a class="button primary" href="#/opnames/new"><Icon name="plus" /> Opname baru</a>
  </div>

  <div class="table-card">
    <table>
      <thead>
        <tr>
          <th>No.</th>
          <th>Dibuat</th>
          <th>Pemilik</th>
          <th>Status</th>
          <th class="num">Dihitung</th>
          <th class="num">Selisih</th>
          <th>Catatan</th>
        </tr>
      </thead>
      <tbody>
        {#each opnames as o (o.id)}
          <tr class="clickable" onclick={() => (location.hash = `#/opnames/${o.id}`)}>
            <td class="code">#{o.id}</td>
            <td>{dateTime(o.created_at)}</td>
            <td class="strong">
              {#if o.member_id}{o.member_name}{:else}<span class="badge store">Toko</span>{/if}
            </td>
            <td>
              <span class="badge" class:draft={o.status === 'draft'}>{o.status === 'draft' ? 'Draft' : 'Diposting'}</span>
            </td>
            <td class="num">{o.counted} / {o.item_count}</td>
            <td class="num" class:neg={o.diff_count > 0}>{o.diff_count}</td>
            <td>{o.note || '—'}</td>
          </tr>
        {:else}
          <tr><td colspan="7" class="empty">Belum ada stok opname.</td></tr>
        {/each}
      </tbody>
    </table>
  </div>
  {#if param === 'new'}
    <Modal title="Opname baru" onclose={() => (location.hash = '#/opnames')}>
    <form class="modal-form" onsubmit={create}>
        <div class="form-body">
            <p class="modal-hint">Pilih pemilik barang yang akan dihitung</p>
          <label>
            Pemilik barang yang dihitung
            <select bind:value={memberId}>
              <option value={null}>Barang toko</option>
              {#each members.filter((m) => m.is_active) as m (m.id)}
                <option value={m.id}>{m.code} — {m.name}</option>
              {/each}
            </select>
          </label>
          <label>Catatan <input bind:value={note} placeholder="Opsional" /></label>
          <p class="muted">
            Semua produk aktif milik pemilik ini masuk ke lembar hitung. Produk yang tidak diisi jumlah fisiknya tidak
            diubah saat posting.
          </p>
          {#if error}<p class="error">{error}</p>{/if}
        </div>
        <div class="form-actions">
          <a class="button" href="#/opnames">Batal</a>
          <button class="primary" disabled={busy}>Mulai menghitung</button>
        </div>
      </form>
    </Modal>
  {/if}
{/if}

<style>
  p.muted {
    margin: 0;
  }
</style>
