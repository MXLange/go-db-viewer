// A resposta pertence à aba que iniciou a operação.
export function applyResult(tab, sessionID, result) {
  if (tab.sessionID !== sessionID || tab.disconnected) return false
  tab.result = result
  tab.error = ''
  return true
}
export function disconnectTabs(tabs, sessionID) {
  for (const tab of tabs) {
    if (tab.sessionID === sessionID) {
      tab.disconnected = true
      tab.busy = false
      tab.error = tab.node ? 'A conexão foi encerrada. Conecte novamente e reabra a tabela.' : 'A conexão foi encerrada. Use Conectar ou Vincular conexão para continuar nesta aba.'
    }
  }
}
