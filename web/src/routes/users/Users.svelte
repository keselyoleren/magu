<script lang="ts" module>
  export interface Account {
    id: number
    username: string
    full_name: string
    role: 'admin' | 'cashier'
    is_active: boolean
    created_at: string
  }
</script>

<script lang="ts">
  import Icon from '../../components/Icon.svelte'
  import { api } from '../../lib/api'
  import { dateOnly } from '../../lib/format'
  import { routeParam } from '../../lib/route.svelte'
  import UserForm from './UserForm.svelte'

  const param = $derived(routeParam())
  let users = $state<Account[]>([])

  $effect(() => {
    void param
    api<Account[]>('/api/users').then((u) => (users = u))
  })
</script>

<div class="page-head">
  <div class="titles">
    <h1>Pengguna</h1>
    <p>Akun admin dan kasir yang boleh masuk ke aplikasi</p>
  </div>
  <a class="button primary" href="#/users/new"><Icon name="plus" /> Tambah pengguna</a>
</div>

<div class="table-card">
  <table>
    <thead>
      <tr>
        <th>Nama</th>
        <th>Username</th>
        <th>Peran</th>
        <th>Dibuat</th>
        <th>Status</th>
        <th></th>
      </tr>
    </thead>
    <tbody>
      {#each users as u (u.id)}
        <tr class:inactive={!u.is_active}>
          <td class="strong">{u.full_name}</td>
          <td class="code">{u.username}</td>
          <td>{u.role === 'admin' ? 'Admin' : 'Kasir'}</td>
          <td>{dateOnly(u.created_at)}</td>
          <td><span class="badge" class:off={!u.is_active}>{u.is_active ? 'Aktif' : 'Nonaktif'}</span></td>
          <td class="num"><a class="row-link" href="#/users/{u.id}">Ubah</a></td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>
{#if param}
  {#key param}
    <UserForm id={param === 'new' ? null : Number(param)} />
  {/key}
{/if}
