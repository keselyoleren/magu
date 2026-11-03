<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '../../components/Icon.svelte'
  import { catalog, loadCatalog } from '../../lib/catalog.svelte'
  import { rupiah } from '../../lib/format'
  import { route } from '../../lib/route.svelte'
  import { logout, session } from '../../lib/session.svelte'
  import Transactions from '../transactions/Transactions.svelte'
  import PrinterSettings from '../printer/PrinterSettings.svelte'
  import Pos from './Pos.svelte'

  const showHistory = $derived(route.hash === '#/pos/history')
  const printerPage = $derived(route.hash === '#/pos/printer')

  onMount(() => {
    loadCatalog()
    const timer = setInterval(loadCatalog, 5 * 60_000)
    const refresh = () => document.visibilityState === 'visible' && loadCatalog()
    document.addEventListener('visibilitychange', refresh)
    return () => {
      clearInterval(timer)
      document.removeEventListener('visibilitychange', refresh)
    }
  })
</script>

<div class="shell">
  <header>
    <span class="mark">magu</span>
    <nav>
      <a href="#/pos" class:current={!showHistory}><Icon name="register" /> Kasir</a>
      <a href="#/pos/history" class:current={showHistory}><Icon name="receipt" /> Riwayat</a>
    </nav>
    <span class="status" class:bad={!!catalog.error} title={catalog.error}>
      {catalog.error ? 'Katalog gagal diperbarui' : catalog.ready ? `${rupiah(catalog.count)} produk` : 'Memuat katalog…'}
    </span>
    <a class="admin" href="#/pos/printer">Printer</a>
    {#if session.user?.role === 'admin'}
      <a class="admin" href="#/home">Panel admin</a>
    {/if}
    <span class="user">{session.user?.full_name}</span>
    <button onclick={logout} aria-label="Keluar" title="Keluar"><Icon name="logout" /></button>
  </header>

  {#if printerPage}
    <div class="history">
      <a class="back" href="#/pos">← Kembali ke kasir</a>
      <PrinterSettings />
    </div>
  {:else}
    <!-- Pos tetap terpasang saat membuka Riwayat supaya keranjang yang sedang diisi tidak hilang. -->
    <div class="body" hidden={showHistory}><Pos /></div>
    {#if showHistory}
      <div class="history"><Transactions /></div>
    {/if}
  {/if}
</div>

<style>
  .shell {
    display: grid;
    grid-template-rows: auto 1fr;
    height: 100vh;
  }
  header {
    display: flex;
    align-items: center;
    gap: 1.2rem;
    padding: 0.55rem 1.2rem;
    color: #c9d6cf;
    background: var(--ink);
  }
  .mark {
    font-family: var(--display);
    font-size: 1.4rem;
    font-weight: 600;
    font-style: italic;
    letter-spacing: -0.03em;
    color: #fff;
  }
  .mark::after {
    content: '.';
    color: var(--amber);
  }
  nav {
    display: flex;
    gap: 0.25rem;
    margin-right: auto;
  }
  nav a {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.45rem 0.8rem;
    border-radius: 7px;
    color: inherit;
    text-decoration: none;
  }
  nav a.current {
    background: rgba(255, 255, 255, 0.1);
    color: #fff;
    font-weight: 500;
  }
  .status {
    font-family: var(--mono);
    font-size: 0.75rem;
    color: #7f958b;
  }
  .status.bad {
    color: var(--amber);
  }
  .admin {
    color: #c9d6cf;
  }
  .user {
    color: #fff;
  }
  header button {
    width: 34px;
    height: 34px;
    padding: 0;
    border-color: transparent;
    background: none;
    color: #9db0a7;
  }
  .body {
    min-height: 0;
  }
  .history {
    overflow-y: auto;
    padding: 1.8rem 2.2rem;
  }
</style>
