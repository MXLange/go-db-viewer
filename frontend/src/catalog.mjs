export function createTableCatalog(load, { ttl = 60000, now = Date.now } = {}) {
  const entries = new Map()
  return {
    async get(sessionID) {
      let entry = entries.get(sessionID)
      if (!entry || entry.expires <= now()) {
        entry = { expires: Infinity }
        entries.set(sessionID, entry)
        entry.promise = Promise.resolve().then(() => load(sessionID)).then(tables => {
          entry.expires = now() + ttl
          return tables || []
        }).catch(error => { entry.failedCurrent = entries.get(sessionID) === entry; if (entry.failedCurrent) entries.delete(sessionID); throw error })
      }
      try {
        const tables = await entry.promise
        return entries.get(sessionID) === entry ? tables : null
      } catch (error) { if (!entry.failedCurrent) return null; throw error }
    },
    invalidate(sessionID) { entries.delete(sessionID) },
  }
}

// Cada sessão tem sua própria geração. Respostas anteriores à invalidação são descartadas.
export function createMetadataCatalog(api, options) {
  const sessions = new Map()
  const cacheFor = id => {
    if (!sessions.has(id)) sessions.set(id, createTableCatalog(async key => {
      const [action, value] = JSON.parse(key)
      return action === 'resolve' ? api.ResolveTable(id, value) : action === 'columns' ? api.Describe(id, value) : api.GetTableProperties(id, value)
    }, options))
    return sessions.get(id)
  }
  async function get(id, action, value) {
    const cache = cacheFor(id)
    try {
      const result = await cache.get(JSON.stringify([action, value]))
      return sessions.get(id) === cache ? result : null
    } catch (error) { if (sessions.get(id) !== cache) return null; throw error }
  }
  return {
    resolve: (id, ref) => get(id, 'resolve', ref),
    properties: (id, node) => get(id, 'properties', node),
    async columns(id, ref) {
      const cache = cacheFor(id), node = await get(id, 'resolve', ref)
      if (!node || sessions.get(id) !== cache) return null
      return get(id, 'columns', node)
    },
    invalidate(id) { sessions.delete(id) },
  }
}
