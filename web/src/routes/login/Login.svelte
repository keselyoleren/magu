<script lang="ts">
  import { login } from '../../lib/session.svelte'

  let username = $state('')
  let password = $state('')
  let error = $state('')
  let busy = $state(false)

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      await login(username, password)
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }
</script>

<div class="split">
  <section class="cover">
    <span class="mark">magu</span>
    <p>Kasir dan pembagian hasil titipan, dalam satu buku.</p>
    <small>kasir &amp; konsinyasi</small>
  </section>

  <form onsubmit={submit}>
    <h1>Masuk</h1>
    <p class="muted">Gunakan akun yang diberikan pemilik toko.</p>
    <label>
      Username
      <!-- svelte-ignore a11y_autofocus -->
      <input bind:value={username} autocomplete="username" required autofocus />
    </label>
    <label>
      Password
      <input type="password" bind:value={password} autocomplete="current-password" required />
    </label>
    {#if error}<p class="error">{error}</p>{/if}
    <button class="primary" disabled={busy}>{busy ? 'Memeriksa…' : 'Masuk'}</button>
  </form>
</div>

<style>
  .split {
    display: grid;
    grid-template-columns: minmax(320px, 5fr) 7fr;
    min-height: 100vh;
  }

  .cover {
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
    padding: 3rem;
    color: #c9d6cf;
    background:
      repeating-linear-gradient(90deg, transparent 0 61px, rgba(255, 255, 255, 0.03) 61px 62px),
      repeating-linear-gradient(0deg, transparent 0 31px, rgba(255, 255, 255, 0.022) 31px 32px),
      linear-gradient(160deg, var(--ink-soft) 0%, var(--ink) 55%, #0a1611 100%);
  }

  .mark {
    font-family: var(--display);
    font-size: 4.2rem;
    font-weight: 600;
    font-style: italic;
    letter-spacing: -0.04em;
    line-height: 1;
    color: #fff;
  }

  .mark::after {
    content: '.';
    color: var(--amber);
  }

  .cover p {
    max-width: 22ch;
    margin: 1.2rem 0 2.5rem;
    font-family: var(--display);
    font-size: 1.35rem;
    line-height: 1.3;
  }

  .cover small {
    font-family: var(--mono);
    font-size: 0.7rem;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: #7f958b;
  }

  form {
    display: grid;
    gap: 1rem;
    align-content: center;
    width: min(340px, 100% - 3rem);
    margin: 0 auto;
    animation: rise 0.3s ease-out;
  }

  form .muted {
    margin: -0.6rem 0 0.4rem;
  }

  button {
    height: 40px;
    margin-top: 0.3rem;
  }

  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(8px);
    }
  }

  @media (max-width: 760px) {
    .split {
      grid-template-columns: 1fr;
      grid-template-rows: auto 1fr;
    }
    .cover {
      padding: 2rem 1.5rem;
    }
    .cover p {
      display: none;
    }
    .mark {
      font-size: 2.6rem;
      margin-bottom: 0.6rem;
    }
  }
</style>
