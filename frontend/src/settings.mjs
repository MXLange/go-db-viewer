/** @returns {import('./api').Settings} */
export const defaultSettings = () => ({ theme: 'system', maxRows: 200 })
export function parseRowLimit(value) {
  const text = String(value).trim()
  if (!/^\d+$/.test(text)) throw new Error('Informe um limite inteiro entre 1 e 100.000.')
  const limit = Number(text)
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100000) throw new Error('Informe um limite inteiro entre 1 e 100.000.')
  return limit
}
export const effectiveRowLimit = (tab, settings) => tab.rowLimit ?? settings.maxRows
export const previewPage = (tab, page, limit) => tab.result && tab.result.rowLimit !== limit ? 0 : page
export const resultItemCount = result => (result?.documents?.length || 0) + (result?.sets?.reduce((sum, set) => sum + set.rows.length, 0) || 0)
export function resultCounter(result) {
  if (!result?.rowLimit) return ''
  const count = resultItemCount(result)
  return `${count}/${result.rowLimit}${count >= result.rowLimit ? ' máx' : ''}`
}
export const effectiveTheme = (preference, systemDark) => preference === 'system' ? (systemDark ? 'dark' : 'light') : preference
export function applyTheme(theme, root = document.documentElement) {
  root.dataset.theme = theme
  root.style.colorScheme = theme
}
