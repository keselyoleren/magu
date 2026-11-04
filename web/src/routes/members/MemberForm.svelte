<script lang="ts">
  import Icon from '../../components/Icon.svelte'
  import Modal from '../../components/Modal.svelte'
  import { del, post, put } from '../../lib/api'
  import { focusFirst } from '../../lib/focus'
  import { bpsToPercent, percentToBps } from '../../lib/format'
  import { listMembers, type Member } from '../../lib/members'
  import { loadSettings } from '../../lib/sales'

  let { id }: { id: number | null } = $props()

  let form = $state<Member>({
    id: 0,
    code: '',
    name: '',
    phone: '',
    email: '',
    address: '',
    bank_name: '',
    bank_account_number: '',
    bank_account_name: '',
    commission_bps: 0,
    is_active: true,
    drive_folder_id: '',
  })
  let commission = $state('')
  let ready = $state(false)
  let error = $state('')
  let busy = $state(false)

  async function load() {
    if (id !== null) {
      const member = (await listMembers()).find((m) => m.id === id)
      if (!member) {
        error = 'Member tidak ditemukan.'
        return
      }
      form = member
      commission = bpsToPercent(member.commission_bps)
    }
    ready = true
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    const bps = percentToBps(commission)
    if (bps === null) {
      error = 'Komisi harus antara 0 dan 100%'
      return
    }
    busy = true
    error = ''
    try {
      const data = { ...form, commission_bps: bps }
      const saved = await (id === null ? post<Member>('/api/members', data) : put<Member>(`/api/members/${id}`, data))
      id = saved.id
      if ((await loadSettings()).drive_enabled) await syncFolder(saved.id)
      location.hash = '#/members'
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  async function syncFolder(memberId: number) {
    try {
      await post(`/api/members/${memberId}/drive`)
    } catch (err) {
      throw new Error(`Member tersimpan, tetapi folder Drive belum diperbarui: ${(err as Error).message}`)
    }
  }

  async function remove() {
    if (!confirm(`Hapus member "${form.name}"? Tindakan ini tidak bisa dibatalkan.`)) return
    busy = true
    error = ''
    try {
      await del(`/api/members/${id}`)
      location.hash = '#/members'
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  load()
</script>

<Modal title={id === null ? 'Member baru' : 'Ubah member'} onclose={() => (location.hash = '#/members')}>
  {#if ready}
    <form class="modal-form" onsubmit={save} use:focusFirst>
      <div class="form-body">
        <p class="modal-hint">Profil penitip, komisi toko, dan rekening untuk pembayaran hasil</p>
        <div class="pair">
          <label>Kode <input bind:value={form.code} required /></label>
          <label>Komisi toko (%) <input bind:value={commission} inputmode="decimal" placeholder="mis. 20" required /></label>
        </div>
        <label>Nama <input bind:value={form.name} required /></label>
        <label>Email Google <input type="email" bind:value={form.email} placeholder="Untuk membuka folder laporan di Drive" /></label>
        <div class="pair">
          <label>Telepon <input bind:value={form.phone} /></label>
          <label>Alamat <input bind:value={form.address} /></label>
        </div>
        <p class="section">Rekening pembayaran</p>
        <div class="pair">
          <label>Bank <input bind:value={form.bank_name} /></label>
          <label>Nomor rekening <input bind:value={form.bank_account_number} /></label>
        </div>
        <label>Atas nama <input bind:value={form.bank_account_name} /></label>
        <label class="check"><input type="checkbox" bind:checked={form.is_active} /> Aktif</label>
        {#if error}<p class="error">{error}</p>{/if}
      </div>
      <div class="form-actions">
        {#if id !== null}
          <button type="button" class="delete" disabled={busy} onclick={remove}><Icon name="trash" /> Hapus</button>
        {/if}
        <a class="button" href="#/members">Batal</a>
        <button class="primary" disabled={busy}>Simpan</button>
      </div>
    </form>
  {:else if error}
    <p class="error">{error}</p>
  {/if}
</Modal>

<style>
  .delete {
    margin-right: auto;
    color: var(--danger);
  }
</style>
