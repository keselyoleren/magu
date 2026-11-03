<script lang="ts">
  import Modal from '../../components/Modal.svelte'
  import { api, post, put } from '../../lib/api'
  import { focusFirst } from '../../lib/focus'
  import { session } from '../../lib/session.svelte'
  import type { Account } from './Users.svelte'

  let { id }: { id: number | null } = $props()

  let username = $state('')
  let fullName = $state('')
  let role = $state<Account['role']>('cashier')
  let isActive = $state(true)
  let password = $state('')
  let ready = $state(false)
  let error = $state('')
  let busy = $state(false)

  const isSelf = $derived(id !== null && id === session.user?.id)

  async function load() {
    if (id !== null) {
      const user = (await api<Account[]>('/api/users')).find((u) => u.id === id)
      if (!user) {
        error = 'Pengguna tidak ditemukan.'
        return
      }
      username = user.username
      fullName = user.full_name
      role = user.role
      isActive = user.is_active
    }
    ready = true
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      const data = { username, full_name: fullName, role, is_active: isActive, password }
      await (id === null ? post('/api/users', data) : put(`/api/users/${id}`, data))
      location.hash = '#/users'
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  load()
</script>

<Modal title={id === null ? 'Pengguna baru' : 'Ubah pengguna'} onclose={() => (location.hash = '#/users')}>
  {#if ready}
    <form class="modal-form" onsubmit={save} use:focusFirst>
      <div class="form-body">
        <p class="modal-hint">Mengubah password atau menonaktifkan akun langsung mengeluarkan pengguna itu dari semua perangkat</p>
        <label>Nama lengkap <input bind:value={fullName} required /></label>
        <div class="pair">
          <label>Username <input bind:value={username} autocomplete="off" required /></label>
          <label>
            Peran
            <select bind:value={role} disabled={isSelf}>
              <option value="cashier">Kasir — hanya layar kasir</option>
              <option value="admin">Admin — semua menu</option>
            </select>
          </label>
        </div>
        <label>
          {id === null ? 'Password' : 'Password baru'}
          <input
            type="password"
            bind:value={password}
            autocomplete="new-password"
            minlength="8"
            required={id === null}
            placeholder={id === null ? 'Minimal 8 karakter' : 'Kosongkan bila tidak diganti'}
          />
        </label>
        <label class="check"><input type="checkbox" bind:checked={isActive} disabled={isSelf} /> Aktif</label>
        {#if isSelf}<p class="muted">Peran dan status akun sendiri tidak bisa diubah dari sini.</p>{/if}
        {#if error}<p class="error">{error}</p>{/if}
      </div>
      <div class="form-actions">
        <a class="button" href="#/users">Batal</a>
        <button class="primary" disabled={busy}>Simpan</button>
      </div>
    </form>
  {:else if error}
    <p class="error">{error}</p>
  {/if}
</Modal>

<style>
  p.muted {
    margin: 0;
  }
</style>
