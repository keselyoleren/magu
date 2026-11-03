export interface DocDraft {
  memberId: number | null
  note: string
  qty: Record<number, number>
}

const key = (kind: string) => `draft-${kind}`

// Disimpan per komputer: yang dilindungi adalah isian scan panjang dari jendela tertutup, refresh, atau listrik mati.
export function readDraft(kind: string): DocDraft | null {
  try {
    return JSON.parse(localStorage.getItem(key(kind)) ?? 'null')
  } catch {
    return null
  }
}

export function writeDraft(kind: string, draft: DocDraft) {
  try {
    if (Object.keys(draft.qty).length > 0) localStorage.setItem(key(kind), JSON.stringify(draft))
    else localStorage.removeItem(key(kind))
  } catch {
    // Tanpa penyimpanan browser, form tetap jalan; hanya isiannya tidak bertahan setelah ditutup.
  }
}

export const clearDraft = (kind: string) => writeDraft(kind, { memberId: null, note: '', qty: {} })
