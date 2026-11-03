<script lang="ts">
  import { onMount, type Snippet } from 'svelte'
  import Icon from './Icon.svelte'

  let { title, onclose, children }: { title: string; onclose: () => void; children: Snippet } = $props()
  let panel: HTMLElement

  onMount(() => panel.querySelector('input')?.focus())
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

<div class="overlay" onclick={onclose} role="presentation"></div>
<aside bind:this={panel} aria-label={title}>
  <header>
    <h2>{title}</h2>
    <button class="link" onclick={onclose} aria-label="Tutup"><Icon name="close" /></button>
  </header>
  {@render children()}
</aside>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 20;
    background: rgba(15, 31, 26, 0.35);
    animation: fade 0.15s ease-out;
  }
  aside {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    z-index: 21;
    width: min(440px, 100vw);
    display: flex;
    flex-direction: column;
    background: var(--card);
    border-left: 1px solid var(--line-strong);
    box-shadow: -18px 0 40px rgba(15, 31, 26, 0.16);
    animation: slide 0.2s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1.1rem 1.4rem;
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
  @keyframes slide {
    from {
      transform: translateX(24px);
      opacity: 0;
    }
  }
</style>
