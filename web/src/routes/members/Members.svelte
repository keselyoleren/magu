<script lang="ts">
  import Icon from '../../components/Icon.svelte'
  import { bpsToPercent } from '../../lib/format'
  import { driveFolderUrl, listMembers, type Member } from '../../lib/members'
  import { routeParam } from '../../lib/route.svelte'
  import { post } from '../../lib/api'
  import { loadSettings } from '../../lib/sales'
  import MemberForm from './MemberForm.svelte'

  const param = $derived(routeParam())

  let members = $state<Member[]>([])
  let search = $state('')
  let driveEnabled = $state(false)
  let error = $state('')

  const shown = $derived.by(() => {
    const q = search.trim().toLowerCase()
    return q ? members.filter((m) => m.name.toLowerCase().includes(q) || m.code.toLowerCase().includes(q)) : members
  })

  $effect(() => {
    void param
    listMembers().then((m) => (members = m))
  })

  loadSettings().then((s) => (driveEnabled = s.drive_enabled))

  async function createFolder(m: Member) {
    error = ''
    try {
      const { folder_id } = await post<{ folder_id: string }>(`/api/members/${m.id}/drive`)
      m.drive_folder_id = folder_id
    } catch (err) {
      error = (err as Error).message
    }
  }
</script>

<div class="page-head">
  <div class="titles">
    <h1>Member</h1>
    <p>{members.length} penitip terdaftar</p>
  </div>
  <a class="button primary" href="#/members/new"><Icon name="plus" /> Tambah member</a>
</div>

<div class="filters">
  <div class="search">
    <Icon name="search" />
    <input placeholder="Cari nama atau kode" bind:value={search} />
  </div>
</div>

{#if error}<p class="error">{error}</p>{/if}

<div class="table-card">
  <table>
    <thead>
      <tr>
        <th>Kode</th>
        <th>Nama</th>
        <th>Telepon</th>
        <th>Rekening</th>
        <th class="num">Komisi toko</th>
        <th>Status</th>
        {#if driveEnabled}<th>Drive</th>{/if}
        <th></th>
      </tr>
    </thead>
    <tbody>
      {#each shown as m (m.id)}
        <tr class:inactive={!m.is_active}>
          <td class="code">{m.code}</td>
          <td class="strong">{m.name}</td>
          <td>{m.phone || '—'}</td>
          <td>{[m.bank_name, m.bank_account_number, m.bank_account_name].filter(Boolean).join(' · ') || '—'}</td>
          <td class="num">{bpsToPercent(m.commission_bps)}%</td>
          <td><span class="badge" class:off={!m.is_active}>{m.is_active ? 'Aktif' : 'Nonaktif'}</span></td>
          {#if driveEnabled}
            <td>
              {#if m.drive_folder_id}
                <a class="row-link" href={driveFolderUrl(m.drive_folder_id)} target="_blank" rel="noopener">Buka folder</a>
              {:else}
                <button onclick={() => createFolder(m)}>Buat folder</button>
              {/if}
            </td>
          {/if}
          <td class="num"><a class="row-link" href="#/members/{m.id}">Ubah</a></td>
        </tr>
      {:else}
        <tr><td colspan={driveEnabled ? 8 : 7} class="empty">{members.length ? 'Tidak ada yang cocok.' : 'Belum ada member.'}</td></tr>
      {/each}
    </tbody>
  </table>
</div>
{#if param}
  {#key param}
    <MemberForm id={param === 'new' ? null : Number(param)} />
  {/key}
{/if}
