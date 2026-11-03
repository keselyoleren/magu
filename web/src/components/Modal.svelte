<script lang="ts">
  import { onMount, type Snippet } from 'svelte'
  import Icon from './Icon.svelte'

  let { title, wide = false, onclose, children }: { title: string; wide?: boolean; onclose: () => void; children: Snippet } =
    $props()

  // Halaman di belakang tidak ikut tergulir selama modal terbuka.
  onMount(() => {
    document.body.style.overflow = 'hidden'
    return () => (document.body.style.overflow = '')
  })
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

<!-- Klik di luar sengaja tidak menutup: isian form yang panjang tidak boleh hilang karena salah klik. -->
<div class="overlay">
  <div class="modal" class:wide role="dialog" aria-modal="true" aria-label={title}>
    <header>
      <h2>{title}</h2>
      <button class="link" onclick={onclose} aria-label="Tutup" title="Tutup (Esc)"><Icon name="close" /></button>
    </header>
    {@render children()}
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 40;
    display: grid;
    place-items: center;
    padding: 1.5rem;
    background: rgba(15, 31, 26, 0.45);
    animation: fade 0.14s ease-out;
  }
  .modal {
    display: flex;
    flex-direction: column;
    width: min(580px, 100%);
    max-height: min(88vh, 760px);
    background: var(--card);
    border: 1px solid var(--line-strong);
    border-radius: 12px;
    box-shadow: 0 24px 60px rgba(15, 31, 26, 0.28);
    animation: pop 0.18s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .modal.wide {
    width: min(780px, 100%);
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.4rem;
    border-bottom: 1px solid var(--line);
  }
  header button {
    color: var(--muted);
  }
  @keyframes fade {
    from {
      opacity: 0;
    }
  }
  @keyframes pop {
    from {
      opacity: 0;
      transform: translateY(10px) scale(0.985);
    }
  }
  @media (max-width: 640px) {
    .overlay {
      padding: 0;
      place-items: end stretch;
    }
    .modal,
    .modal.wide {
      width: 100%;
      max-height: 94vh;
      border-radius: 14px 14px 0 0;
    }
  }
</style>
