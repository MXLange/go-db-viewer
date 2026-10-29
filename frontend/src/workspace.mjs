// Somente referências a scripts confirmados entram no estado das abas.
export function serializeWorkspace(tabs, activeTabID) {
  const openTabs = tabs.filter(tab => !tab.node && tab.scriptID).map(tab => ({
    id: tab.id, scriptID: tab.scriptID, anchor: tab.anchor || 0, head: tab.head || 0,
  }))
  return { version: 2, activeTabID: openTabs.some(tab => tab.id === activeTabID) ? activeTabID : openTabs[0]?.id || 0, openTabs }
}

export function restoreWorkspace(workspace) {
  const scripts = new Map(workspace.scripts.map(script => [script.id, script]))
  return workspace.openTabs.filter(tab => scripts.has(tab.scriptID)).map(tab => {
    const script = scripts.get(tab.scriptID)
    return { ...script, ...tab, title: script.title,
      anchor: Math.min(script.text.length, tab.anchor), head: Math.min(script.text.length, tab.head),
      sessionID: '', page: 0, columns: [], busy: false, error: '', disconnected: true,
    }
  })
}

export function scriptDirty(tab, scripts) {
  if (tab.node) return false
  const saved = scripts.find(script => script.id === tab.scriptID)
  return saved ? tab.text !== saved.text || tab.database !== saved.database || tab.profileID !== saved.profileID
    : tab.text !== tab.initialText || tab.database !== (tab.initialDatabase ?? tab.database)
}
export function scriptSnapshot(tab, title) {
  return { id: tab.scriptID || '', title, text: tab.text, kind: tab.kind,
    profileID: tab.profileID || '', connectionName: tab.connectionName, database: tab.database }
}
export function scriptList(scripts, profiles, filter, search = '') {
  const ids = new Set(profiles.map(profile => profile.id)), query = search.trim().toLocaleLowerCase()
  return scripts.filter(script => (filter === '$all' || filter === '$unlinked' && !ids.has(script.profileID) || script.profileID === filter)
    && script.title.toLocaleLowerCase().includes(query)).toSorted((a,b) => b.updatedAt.localeCompare(a.updatedAt) || a.title.localeCompare(b.title))
}

// As gravações seguem a ordem das revisões, mesmo com respostas lentas.
export function createWorkspaceSaver(save, onStatus, { delay = 500, setTimer = setTimeout, clearTimer = clearTimeout } = {}) {
  let timer, latest, revision = 0, savedRevision = 0, queuedRevision = -1
  let queue = Promise.resolve()
  function enqueue() {
    if (!latest || savedRevision === revision || queuedRevision === revision) return queue
    const snapshot = structuredClone(latest), current = revision
    queuedRevision = current
    queue = queue.catch(() => {}).then(() => save(snapshot)).then(() => {
      savedRevision = current
      if (current === revision) onStatus('Salvo')
    }).catch(error => {
      if (current === revision) onStatus('Falha ao salvar', error)
      throw error
    }).finally(() => { if (queuedRevision === current) queuedRevision = -1 })
    return queue
  }
  return {
    schedule(workspace) {
      latest = structuredClone(workspace); revision++
      clearTimer(timer); onStatus('Salvando')
      timer = setTimer(() => { timer = undefined; void enqueue().catch(() => {}) }, delay)
    },
    flush() { clearTimer(timer); timer = undefined; return enqueue() },
    dispose() { clearTimer(timer) },
  }
}
