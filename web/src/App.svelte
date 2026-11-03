<script lang="ts">
  import type { Component } from 'svelte'
  import Icon, { type IconName } from './components/Icon.svelte'
  import { route } from './lib/route.svelte'
  import { loadSession, logout, session } from './lib/session.svelte'
  import Login from './routes/login/Login.svelte'
  import Members from './routes/members/Members.svelte'
  import Opnames from './routes/opnames/Opnames.svelte'
  import PrinterSettings from './routes/printer/PrinterSettings.svelte'
  import PosShell from './routes/pos/PosShell.svelte'
  import Transactions from './routes/transactions/Transactions.svelte'
  import PriceEditor from './routes/products/PriceEditor.svelte'
  import StockEditor from './routes/products/StockEditor.svelte'
  import Products from './routes/products/Products.svelte'
  import Home from './routes/home/Home.svelte'
  import Labels from './routes/labels/Labels.svelte'
  import Receipts from './routes/receipts/Receipts.svelte'
  import StockReport from './routes/reports/StockReport.svelte'
  import Returns from './routes/returns/Returns.svelte'
  import Users from './routes/users/Users.svelte'
  import Reports from './routes/reports/Reports.svelte'
  import Settlements from './routes/settlements/Settlements.svelte'

  const menu: { group: string; items: { hash: string; label: string; icon: IconName; component: Component }[] }[] = [
    {
      group: 'Ringkasan',
      items: [{ hash: '#/home', label: 'Beranda', icon: 'home', component: Home }],
    },
    {
      group: 'Penjualan',
      items: [
        { hash: '#/transactions', label: 'Transaksi', icon: 'receipt', component: Transactions },
        { hash: '#/settlements', label: 'Bagi hasil', icon: 'wallet', component: Settlements },
        { hash: '#/reports', label: 'Laporan', icon: 'chart', component: Reports },
      ],
    },
    {
      group: 'Barang',
      items: [
        { hash: '#/products', label: 'Produk', icon: 'box', component: Products },
        { hash: '#/receipts', label: 'Terima barang', icon: 'inbox', component: Receipts },
        { hash: '#/labels', label: 'Cetak label', icon: 'tag', component: Labels },
        { hash: '#/prices', label: 'Atur harga', icon: 'edit', component: PriceEditor },
        { hash: '#/stocks', label: 'Atur stok', icon: 'sliders', component: StockEditor },
        { hash: '#/opnames', label: 'Stok opname', icon: 'clipboard', component: Opnames },
        { hash: '#/returns', label: 'Retur ke penitip', icon: 'undo', component: Returns },
        { hash: '#/stock-report', label: 'Stok titipan', icon: 'layers', component: StockReport },
      ],
    },
    {
      group: 'Master data',
      items: [
        { hash: '#/members', label: 'Member', icon: 'users', component: Members },
        { hash: '#/users', label: 'Pengguna', icon: 'key', component: Users },
        { hash: '#/printer', label: 'Printer', icon: 'printer', component: PrinterSettings },
      ],
    },
  ]
  const pages = menu.flatMap((g) => g.items)

  let menuOpen = $state(false)
  const page = $derived(pages.find((p) => route.hash === p.hash || route.hash.startsWith(p.hash + '/')) ?? pages[0])
  const isAdmin = $derived(session.user?.role === 'admin')
  const initials = $derived(
    (session.user?.full_name ?? '')
      .split(' ')
      .slice(0, 2)
      .map((w) => w[0])
      .join('')
      .toUpperCase(),
  )

  $effect(() => {
    route.hash
    menuOpen = false
  })

  loadSession()
</script>

{#if !session.ready}
  <p class="loading">Memuat…</p>
{:else if !session.user}
  <Login />
{:else if !isAdmin || route.hash.startsWith('#/pos')}
  <PosShell />
{:else}
  <div class="shell">
    <aside class:open={menuOpen}>
      <a class="brand" href={pages[0].hash}>
        <span class="mark">magu</span>
        <span class="tag">kasir &amp; konsinyasi</span>
      </a>

      <nav>
        <a class="open-pos" href="#/pos"><Icon name="register" /> Layar kasir</a>
          {#each menu as section}
            <p class="group">{section.group}</p>
            {#each section.items as item}
              <a href={item.hash} class:current={item === page} aria-current={item === page ? 'page' : undefined}>
                <Icon name={item.icon} />
                {item.label}
              </a>
            {/each}
          {/each}
      </nav>

      <div class="account">
        <span class="avatar">{initials}</span>
        <span class="who">
          <strong>{session.user.full_name}</strong>
          <small>{isAdmin ? 'Admin' : 'Kasir'}</small>
        </span>
        <button class="signout" onclick={logout} aria-label="Keluar" title="Keluar"><Icon name="logout" /></button>
      </div>
    </aside>

    {#if menuOpen}
      <div class="scrim" onclick={() => (menuOpen = false)} role="presentation"></div>
    {/if}

    <div class="content">
      <div class="mobile-bar">
        <button class="link" onclick={() => (menuOpen = true)} aria-label="Buka menu"><Icon name="menu" size={22} /></button>
        <span class="mark">magu</span>
      </div>
      <main>
        {#key page}
          <div class="page"><page.component /></div>
        {/key}
      </main>
    </div>
  </div>
{/if}

<style>
  .loading {
    margin-top: 30vh;
    text-align: center;
    color: var(--muted);
  }

  .shell {
    display: grid;
    grid-template-columns: 248px 1fr;
    min-height: 100vh;
  }

  aside {
    position: sticky;
    top: 0;
    height: 100vh;
    display: flex;
    flex-direction: column;
    padding: 1.4rem 0 0.9rem;
    color: #c9d6cf;
    /* garis kolom buku kas, nyaris tak terlihat */
    background:
      repeating-linear-gradient(90deg, transparent 0 61px, rgba(255, 255, 255, 0.028) 61px 62px),
      linear-gradient(180deg, var(--ink) 0%, #0b1813 100%);
  }

  .brand {
    display: grid;
    padding: 0 1.5rem 1.5rem;
    color: inherit;
    text-decoration: none;
  }

  .mark {
    font-family: var(--display);
    font-size: 1.75rem;
    font-weight: 600;
    font-style: italic;
    letter-spacing: -0.03em;
    line-height: 1;
    color: #fff;
  }

  .mark::after {
    content: '.';
    color: var(--amber);
  }

  .tag {
    margin-top: 0.35rem;
    font-family: var(--mono);
    font-size: 0.68rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: #7f958b;
  }

  nav {
    flex: 1;
    display: grid;
    align-content: start;
    gap: 2px;
    padding: 0 0.9rem;
    overflow-y: auto;
  }

  .group {
    margin: 0.9rem 0 0.35rem;
    padding: 0 0.6rem;
    font-family: var(--mono);
    font-size: 0.66rem;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: #6f867b;
  }

  nav a {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.7rem;
    padding: 0.55rem 0.6rem;
    border-radius: 7px;
    color: inherit;
    text-decoration: none;
    transition:
      background 0.12s,
      color 0.12s;
  }

  nav a:hover {
    background: rgba(255, 255, 255, 0.05);
    color: #fff;
  }

  nav a.current {
    background: rgba(255, 255, 255, 0.08);
    color: #fff;
    font-weight: 500;
  }

  nav a.open-pos {
    justify-content: center;
    margin-bottom: 0.3rem;
    border: 1px solid rgba(214, 150, 26, 0.55);
    color: #f1d9a3;
    font-weight: 500;
  }

  nav a.open-pos:hover {
    background: rgba(214, 150, 26, 0.14);
    color: #fff;
  }

  nav a.current::before {
    content: '';
    position: absolute;
    left: -0.9rem;
    top: 7px;
    bottom: 7px;
    width: 3px;
    border-radius: 0 3px 3px 0;
    background: var(--amber);
  }

  .account {
    display: flex;
    align-items: center;
    gap: 0.65rem;
    margin: 0 0.9rem;
    padding: 0.7rem 0.6rem 0.2rem;
    border-top: 1px solid rgba(255, 255, 255, 0.09);
  }

  .avatar {
    display: grid;
    place-items: center;
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: var(--ink-soft);
    border: 1px solid rgba(255, 255, 255, 0.14);
    font-family: var(--mono);
    font-size: 0.72rem;
    color: #fff;
  }

  .who {
    display: grid;
    min-width: 0;
    margin-right: auto;
    line-height: 1.25;
  }

  .who strong {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 500;
    color: #fff;
  }

  .who small {
    color: #7f958b;
  }

  .signout {
    width: 32px;
    height: 32px;
    padding: 0;
    border-color: transparent;
    background: none;
    color: #9db0a7;
  }

  .signout:hover:not(:disabled) {
    border-color: rgba(255, 255, 255, 0.2);
    color: #fff;
  }

  .content {
    min-width: 0;
  }

  main {
    max-width: 1180px;
    margin: 0 auto;
    padding: 2.2rem 2.4rem 3rem;
  }

  .page {
    animation: rise 0.22s ease-out;
  }

  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
  }

  .mobile-bar,
  .scrim {
    display: none;
  }

  @media (max-width: 860px) {
    .shell {
      grid-template-columns: 1fr;
    }
    aside {
      position: fixed;
      z-index: 30;
      width: 248px;
      transform: translateX(-100%);
      transition: transform 0.2s ease-out;
    }
    aside.open {
      transform: none;
    }
    .scrim {
      display: block;
      position: fixed;
      inset: 0;
      z-index: 29;
      background: rgba(15, 31, 26, 0.4);
    }
    .mobile-bar {
      display: flex;
      align-items: center;
      gap: 0.6rem;
      padding: 0.6rem 1rem;
      background: var(--ink);
    }
    .mobile-bar button {
      color: #fff;
    }
    .mobile-bar .mark {
      font-size: 1.3rem;
    }
    main {
      padding: 1.4rem 1rem 2rem;
    }
  }
</style>
