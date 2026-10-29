<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { api, errorMessage, isDesktop, onWorkspaceClose, rootNode, type Column, type ConnectionInput, type DBNode, type DriverInfo, type Kind, type Profile, type Result, type ScriptTab, type TableReference, type TableProperties, type SavedScript, type WorkspaceState } from './api'
import { applyResult, disconnectTabs } from './tabs.mjs'
import { createWorkspaceSaver, serializeWorkspace, restoreWorkspace, scriptDirty, scriptSnapshot } from './workspace.mjs'
import { createTableCatalog, createMetadataCatalog } from './catalog.mjs'
import CodeEditor from './components/CodeEditor.vue'
import TreeNode from './components/TreeNode.vue'
import ResultView from './components/ResultView.vue'
import PropertiesView from './components/PropertiesView.vue'
import ScriptLibrary from './components/ScriptLibrary.vue'

interface Connection { sessionID: string; profileID: string; name: string; kind: Kind; database: string; roots: DBNode[]; treeError: string; loading: boolean }
interface Tab extends ScriptTab { saving?: boolean; scriptError?: string; propertiesView?: boolean; properties?: TableProperties; metadataVersion?: number; sessionID: string; node?: DBNode; page: number; result?: Result; columns: Column[]; busy: boolean; error: string; disconnected: boolean }
const drivers = ref<DriverInfo[]>([]), profiles = ref<Profile[]>([]), connections = ref<Connection[]>([]), tabs = ref<Tab[]>([])
const scripts = ref<SavedScript[]>([]), libraryDialog = ref<HTMLDialogElement>(), libraryProfileID = ref('$all'), libraryVersion = ref(0)
const nameDialog = ref<HTMLDialogElement>(), nameValue = ref(''), nameError = ref(''), nameBusy = ref(false)
const naming = ref<{ tab?: Tab; script?: SavedScript }>()
const changesDialog = ref<HTMLDialogElement>(), changesTab = ref<Tab>(), changesBusy = ref(false), changesError = ref('')
const removeScriptDialog = ref<HTMLDialogElement>(), removingScript = ref<SavedScript>(), removeScriptBusy = ref(false), removeScriptError = ref('')
const closeRequested = ref(false)
const pendingSaves = new Set<Promise<boolean>>()
let nameResolve: ((saved: boolean) => void) | undefined, changesResolve: ((proceed: boolean) => void) | undefined
const workspaceReady = ref(false), saveStatus = ref('Carregando scripts'), saveError = ref(''), closing = ref(false)
const linkDialog = ref<HTMLDialogElement>(), linkTabID = ref(0), linkValue = ref(''), linkBusy = ref(false), linkError = ref('')
const selectedSession = ref(''), activeTabID = ref(0), notice = ref(''), noticeError = ref(false), filter = ref('')
const starting = ref(''), globalBusy = ref(false), desktop = ref(isDesktop())
const dialog = ref<HTMLDialogElement>(), deleteDialog = ref<HTMLDialogElement>(), pendingDelete = ref<Profile>()
const form = reactive<ConnectionInput>({ id: '', name: '', kind: 'postgresql', dsn: '' })
const editor = ref<{ executeCurrentCommand(): void }>()
const formError = ref(''), formStatus = ref(''), formBusy = ref(false), reveal = ref(false)
const formDriver = computed(() => drivers.value.find(d => d.kind === form.kind))
const active = computed(() => tabs.value.find(t => t.id === activeTabID.value))
const selected = computed(() => connections.value.find(c => c.sessionID === selectedSession.value))
const filteredProfiles = computed(() => profiles.value.filter(p => p.name.toLowerCase().includes(filter.value.toLowerCase())))
const totalRows = computed(() => active.value?.result?.documents?.length || active.value?.result?.sets?.reduce((sum, set) => sum + set.rows.length, 0) || 0)
let nextID = 1, lastFocus: HTMLElement | null = null
let initialization: Promise<void> = Promise.resolve(), unsubscribeClose = () => {}
const catalog = createTableCatalog((id: string) => api.ListTables(id))
const metadata = createMetadataCatalog(api)
function invalidateMetadata(id: string) { catalog.invalidate(id); metadata.invalidate(id) }
const loadColumns = (id: string, reference: TableReference) => metadata.columns(id, reference)
const loadTables = (id: string) => catalog.get(id)
const saver = createWorkspaceSaver((workspace: WorkspaceState) => api.SaveWorkspace(workspace), (status: string, error?: unknown) => {
  saveStatus.value = status
  if (error) saveError.value = errorMessage(error)
  else if (status === 'Salvo') saveError.value = ''
})
watch(() => serializeWorkspace(tabs.value, activeTabID.value), snapshot => {
  if (workspaceReady.value && !closing.value) saver.schedule(snapshot)
}, { flush: 'sync' })

async function restoreScripts(merge = false) {
  const loaded = await api.LoadWorkspace()
  scripts.value = loaded.workspace.scripts
  const local = merge ? [...tabs.value] : []
  const restored = (restoreWorkspace(loaded.workspace) as Tab[]).filter(tab => !local.some(t => t.scriptID && t.scriptID === tab.scriptID))
  const localActive = local.find(tab => tab.id === activeTabID.value)
  const used = new Set(restored.map(tab => tab.id))
  nextID = Math.max(0, ...restored.map(tab => tab.id), ...local.map(tab => tab.id)) + 1
  for (const tab of local) { if (used.has(tab.id)) tab.id = nextID++; used.add(tab.id) }
  tabs.value = [...restored, ...local]
  activeTabID.value = localActive?.id || loaded.workspace.activeTabID
  workspaceReady.value = true; saveStatus.value = 'Salvo'; saveError.value = ''
  if (loaded.warning) announce(loaded.warning, true)
}
async function retrySaving() {
  try {
    if (!workspaceReady.value) await restoreScripts(true)
    saver.schedule(serializeWorkspace(tabs.value, activeTabID.value))
    await saver.flush()
  } catch (error) { saveStatus.value = 'Falha ao salvar'; saveError.value = errorMessage(error) }
}
async function saveBeforeClose() {
  if (closing.value || closeRequested.value || nameDialog.value?.open || changesDialog.value?.open || removeScriptDialog.value?.open) return
  closeRequested.value = true
  try {
    await initialization
    if (!workspaceReady.value) throw new Error('Não foi possível carregar os scripts anteriores. Corrija o erro e tente novamente.')
    await Promise.all([...pendingSaves])
    for (const tab of [...tabs.value]) if (!await confirmChanges(tab)) return
    closing.value = true
    await saver.flush()
    await api.SaveWorkspaceAndClose(serializeWorkspace(tabs.value, activeTabID.value))
  } catch (error) { closing.value = false; saveStatus.value = 'Falha ao salvar'; saveError.value = errorMessage(error) }
  finally { closeRequested.value = false }
}
function dirty(tab: Tab) { return scriptDirty(tab, scripts.value) }
function scriptStatus(tab: Tab) {
  return tab.saving ? 'Salvando…' : tab.scriptError ? 'Falha ao salvar' : !tab.scriptID ? 'Não salvo' : dirty(tab) ? 'Alterações não salvas' : 'Salvo'
}
function putScript(script: SavedScript) {
  const index = scripts.value.findIndex(item => item.id === script.id)
  if (index < 0) scripts.value.push(script)
  else scripts.value[index] = script
}
function persistScript(tab: Tab, title: string): Promise<boolean> {
  if (tab.node || tab.saving || !workspaceReady.value) return Promise.resolve(false)
  tab.saving = true; tab.scriptError = ''
  const snapshot = scriptSnapshot(tab, title)
  const operation = (async () => {
    try {
      const saved = await api.SaveScript(snapshot)
      putScript(saved); tab.scriptID = saved.id; tab.title = saved.title
      return true
    } catch (error) { tab.scriptError = errorMessage(error); return false }
    finally { tab.saving = false }
  })()
  pendingSaves.add(operation)
  void operation.finally(() => pendingSaves.delete(operation))
  return operation
}
async function saveScript(tab: Tab): Promise<boolean> {
  if (tab.node || tab.saving || !workspaceReady.value) return false
  if (tab.scriptID) return persistScript(tab, tab.title)
  return nameScript({ tab })
}
async function nameScript(target: { tab?: Tab; script?: SavedScript }): Promise<boolean> {
  if (nameDialog.value?.open || target.script && tabs.value.some(tab => tab.scriptID === target.script!.id && tab.saving)) return false
  naming.value = target; nameValue.value = target.script?.title || ''; nameError.value = ''
  const result = new Promise<boolean>(resolve => { nameResolve = resolve })
  await nextTick(); nameDialog.value?.showModal()
  return result
}
function closeName(saved = false) {
  if (nameBusy.value) return
  nameDialog.value?.close(); naming.value = undefined
  const resolve = nameResolve; nameResolve = undefined; resolve?.(saved)
}
async function submitName() {
  const target = naming.value
  if (!target || nameBusy.value) return
  const title = nameValue.value.trim()
  if (!title || [...title].length > 120) { nameError.value = 'Informe um nome com 1 a 120 caracteres.'; return }
  nameBusy.value = true; nameError.value = ''
  try {
    if (target.tab) {
      if (!await persistScript(target.tab, title)) throw new Error(target.tab.scriptError || 'Não foi possível salvar o script.')
    } else if (target.script) {
      const saved = await api.RenameScript(target.script.id, title)
      putScript(saved)
      for (const tab of tabs.value) if (tab.scriptID === saved.id) tab.title = saved.title
    }
    nameBusy.value = false; closeName(true)
  } catch (error) { nameError.value = errorMessage(error) }
  finally { nameBusy.value = false }
}
async function confirmChanges(tab: Tab): Promise<boolean> {
  if (!dirty(tab)) return true
  activeTabID.value = tab.id; changesTab.value = tab; changesError.value = ''
  const result = new Promise<boolean>(resolve => { changesResolve = resolve })
  await nextTick(); changesDialog.value?.showModal()
  return result
}
async function decideChanges(choice: 'save' | 'discard' | 'cancel') {
  if (changesBusy.value) return
  changesBusy.value = true
  if (choice === 'save' && changesTab.value) {
    if (!await saveScript(changesTab.value)) { changesError.value = changesTab.value.scriptError || 'Salvamento não concluído.'; changesBusy.value = false; return }
  }
  changesDialog.value?.close(); changesBusy.value = false; changesTab.value = undefined
  const resolve = changesResolve; changesResolve = undefined; resolve?.(choice !== 'cancel')
}
async function showLibrary(profileID = '$all') {
  if (closing.value || closeRequested.value) return
  libraryProfileID.value = profileID === '$all' || profileID === '$unlinked' || profiles.value.some(profile => profile.id === profileID) ? profileID : '$unlinked'
  libraryVersion.value++
  await nextTick(); libraryDialog.value?.showModal()
}
function closeLibrary() { libraryDialog.value?.close() }
function createFromLibrary(target: { profileID: string; kind: Kind }) {
  closeLibrary()
  const profile = profiles.value.find(p => p.id === target.profileID)
  const connection = connections.value.find(c => c.profileID === target.profileID && target.profileID)
  newTab(connection || { sessionID: '', profileID: profile?.id || '', name: profile?.name || 'Sem conexão salva', kind: profile?.kind || target.kind, database: '', roots: [], treeError: '', loading: false })
}
function openSavedScript(script: SavedScript) {
  closeLibrary()
  const existing = tabs.value.find(tab => tab.scriptID === script.id)
  if (existing) { activeTabID.value = existing.id; return }
  const tab = reactive<Tab>({ ...script, id: nextID++, scriptID: script.id, sessionID: '', anchor: 0, head: 0, page: 0, columns: [], busy: false, error: '', disconnected: true })
  const connection = connections.value.find(c => script.profileID && c.profileID === script.profileID && c.kind === script.kind)
  if (connection) bindTab(tab, connection)
  tabs.value.push(tab); activeTabID.value = tab.id
}
async function requestRemoveScript(script: SavedScript) {
  if (tabs.value.some(tab => tab.scriptID === script.id && (tab.saving || tab.busy))) return
  removingScript.value = script; removeScriptError.value = ''; await nextTick(); removeScriptDialog.value?.showModal()
}
function cancelRemoveScript() { if (!removeScriptBusy.value) { removeScriptDialog.value?.close(); removingScript.value = undefined } }
async function removeScript() {
  const script = removingScript.value; if (!script || removeScriptBusy.value) return
  removeScriptBusy.value = true; removeScriptError.value = ''
  try {
    await saver.flush(); await api.DeleteScript(script.id)
    scripts.value = scripts.value.filter(item => item.id !== script.id)
    for (const tab of [...tabs.value]) if (tab.scriptID === script.id) removeTab(tab)
    removeScriptBusy.value = false; cancelRemoveScript()
  } catch (error) { removeScriptError.value = errorMessage(error) }
  finally { removeScriptBusy.value = false }
}
async function connectScript(tab: Tab) {
  const profile = profiles.value.find(p => p.id === tab.profileID && p.kind === tab.kind)
  if (!profile) { await showLink(tab); return }
  if (starting.value) return
  starting.value = profile.id
  try { const connection = await connectInput({ id: profile.id, name: profile.name, kind: profile.kind, dsn: '' }); bindTab(tab, connection) }
  catch (error) { tab.error = errorMessage(error) }
  finally { starting.value = '' }
}
function bindTab(tab: Tab, connection: Connection) {
  if (tab.kind !== connection.kind) return
  tab.sessionID = connection.sessionID; tab.profileID = connection.profileID; tab.connectionName = connection.name
  tab.database = tab.kind === 'mongodb' ? (tab.database || connection.database) : connection.database
  tab.disconnected = false; tab.error = ''
}
function openConnectionTab(connection: Connection) {
  const tab = tabs.value.find(tab => !tab.node && tab.sessionID === connection.sessionID && !tab.disconnected)
  if (tab) activeTabID.value = tab.id
  else newTab(connection)
}
async function showLink(tab: Tab) {
  linkTabID.value = tab.id; linkError.value = ''
  const connection = connections.value.find(c => c.kind === tab.kind)
  const profile = profiles.value.find(p => p.kind === tab.kind && p.id === tab.profileID) || profiles.value.find(p => p.kind === tab.kind)
  linkValue.value = connection ? `active:${connection.sessionID}` : profile ? `profile:${profile.id}` : ''
  lastFocus = document.activeElement as HTMLElement
  await nextTick(); linkDialog.value?.showModal()
}
function closeLink() { if (!linkBusy.value) { linkDialog.value?.close(); lastFocus?.focus() } }
async function linkSelected() {
  const tab = tabs.value.find(tab => tab.id === linkTabID.value)
  if (!tab || !linkValue.value) return
  linkBusy.value = true; linkError.value = ''
  try {
    let connection: Connection | undefined
    if (linkValue.value.startsWith('active:')) connection = connections.value.find(c => c.sessionID === linkValue.value.slice(7))
    else {
      const profile = profiles.value.find(p => p.id === linkValue.value.slice(8))
      if (profile && profile.kind === tab.kind) connection = await connectInput({ id: profile.id, name: profile.name, kind: profile.kind, dsn: '' })
    }
    if (!connection || connection.kind !== tab.kind) throw new Error('Escolha uma conexão do mesmo tipo de banco.')
    bindTab(tab, connection); selectedSession.value = connection.sessionID
    linkBusy.value = false; closeLink()
  } catch (error) { linkError.value = errorMessage(error) }
  finally { linkBusy.value = false }
}

function announce(message: string, error = false) { notice.value = message; noticeError.value = error }
function driverName(kind: Kind) { return drivers.value.find(d => d.kind === kind)?.name || kind }
function connectionFor(profile: Profile) { return connections.value.find(c => c.profileID === profile.id) }
async function refreshProfiles() { profiles.value = await api.Profiles() || [] }
async function initialize() {
  globalBusy.value = true
  try { drivers.value = await api.Drivers(); await refreshProfiles() }
  catch (e) { announce(errorMessage(e), true) }
  try { await restoreScripts() }
  catch (e) { saveStatus.value = 'Falha ao salvar'; saveError.value = errorMessage(e) }
  finally { globalBusy.value = false }
}
async function openForm(profile?: Profile) {
  if (globalBusy.value || closing.value) return
  lastFocus = document.activeElement as HTMLElement
  Object.assign(form, { id: profile?.id || '', name: profile?.name || '', kind: profile?.kind || 'postgresql', dsn: '' })
  formError.value = ''; formStatus.value = ''; reveal.value = false
  await nextTick(); dialog.value?.showModal()
}
function closeForm() {
  if (formBusy.value) return
  form.dsn = ''; reveal.value = false; dialog.value?.close(); lastFocus?.focus()
}
async function testForm() {
  formBusy.value = true; formError.value = ''; formStatus.value = ''
  try { await api.TestConnection({ ...form }); formStatus.value = 'Conexão testada com sucesso.' }
  catch (e) { formError.value = errorMessage(e) }
  finally { formBusy.value = false }
}
async function saveForm() {
  formBusy.value = true; formError.value = ''; formStatus.value = ''
  try {
    const profile = await api.SaveProfile({ ...form })
	const connected = connections.value.find(c => c.profileID === profile.id)
	if (connected) await disconnect(connected)
    form.id = profile.id; form.dsn = ''; reveal.value = false
    await refreshProfiles(); formStatus.value = 'Conexão salva. A string está no cofre do sistema.'
  } catch (e) { formError.value = errorMessage(e) }
  finally { formBusy.value = false }
}
async function loadRoots(connection: Connection) {
  invalidateMetadata(connection.sessionID)
  connection.loading = true; connection.treeError = ''
  try { connection.roots = await api.List(connection.sessionID, rootNode()) || []; if (connection.kind !== 'mongodb') connection.database = connection.roots[0]?.database || '' }
  catch (e) { connection.treeError = errorMessage(e) }
  finally { connection.loading = false }
}
async function connectInput(input: ConnectionInput) {
  const existing = input.id && connections.value.find(c => c.profileID === input.id)
  if (existing) {
    selectedSession.value = existing.sessionID
    for (const tab of tabs.value) if (!tab.node && tab.disconnected && tab.profileID === existing.profileID && tab.kind === existing.kind) bindTab(tab, existing)
    return existing
  }
  const sessionID = await api.Connect(input)
  const connection = reactive<Connection>({ sessionID, profileID: input.id, name: input.name || driverName(input.kind), kind: input.kind, database: '', roots: [], treeError: '', loading: false })
  connections.value.push(connection); selectedSession.value = sessionID
  await loadRoots(connection)
  if (connection.profileID) for (const tab of tabs.value) {
    if (!tab.node && tab.disconnected && tab.profileID === connection.profileID && tab.kind === connection.kind) bindTab(tab, connection)
  }
  announce(`Conectado a ${connection.name}.`)
  return connection
}
async function connectForm() {
  if (formBusy.value) return
  formBusy.value = true; formError.value = ''
  try { const connection = await connectInput({ ...form }); formBusy.value = false; closeForm(); openConnectionTab(connection) }
  catch (e) { formError.value = errorMessage(e) }
  finally { formBusy.value = false }
}
async function connectProfile(profile: Profile) { await showLibrary(profile.id) }
async function disconnect(connection: Connection) {
  invalidateMetadata(connection.sessionID)
  disconnectTabs(tabs.value, connection.sessionID)
  connections.value = connections.value.filter(c => c !== connection)
  if (selectedSession.value === connection.sessionID) selectedSession.value = connections.value[0]?.sessionID || ''
  try { await api.Disconnect(connection.sessionID); announce(`Conexão ${connection.name} encerrada.`) }
  catch (e) { announce(errorMessage(e), true) }
}
async function requestDelete(profile: Profile) { pendingDelete.value = profile; lastFocus = document.activeElement as HTMLElement; await nextTick(); deleteDialog.value?.showModal() }
function closeDelete() { if (!globalBusy.value) { deleteDialog.value?.close(); pendingDelete.value = undefined; lastFocus?.focus() } }
async function removeProfile() {
  const profile = pendingDelete.value; if (!profile) return
  globalBusy.value = true
  try {
    await api.DeleteProfile(profile.id)
    const connection = connectionFor(profile); if (connection) await disconnect(connection)
    await refreshProfiles(); globalBusy.value = false; closeDelete(); announce('Conexão e credencial removidas.')
  } catch (e) { announce(errorMessage(e), true) }
  finally { globalBusy.value = false }
}
function newTab(connection = selected.value) {
  if (!connection) { void showLibrary(active.value?.profileID || '$all'); return }
  const tab = reactive<Tab>({ id: nextID++, profileID: connection.profileID, anchor: 0, head: 0, sessionID: connection.sessionID, connectionName: connection.name, kind: connection.kind, title: 'Nova consulta', text: connection.kind === 'mongodb' ? '{\n  "find": "colecao",\n  "filter": {},\n  "limit": 100\n}' : '-- Escreva um comando SQL.\n', database: connection.database, page: 0, columns: [], busy: false, error: '', disconnected: !connection.sessionID })
  tab.initialText = tab.text; tab.initialDatabase = tab.database
  tabs.value.push(tab); activeTabID.value = tab.id
}
async function closeTab(tab: Tab) {
  if ((tab.busy && !tab.propertiesView) || tab.saving || closeRequested.value || changesDialog.value?.open) return
  if (!await confirmChanges(tab)) return
  removeTab(tab)
}
function removeTab(tab: Tab) {
  tab.metadataVersion = (tab.metadataVersion || 0) + 1
  const index = tabs.value.indexOf(tab); tabs.value.splice(index, 1)
  if (activeTabID.value === tab.id) activeTabID.value = tabs.value[Math.max(0, index - 1)]?.id || 0
}
function selectDatabase(connection: Connection, database: string) {
  connection.database = database; selectedSession.value = connection.sessionID
  if (active.value?.sessionID === connection.sessionID && !active.value.busy) active.value.database = database
}
async function openNode(connection: Connection, node: DBNode) {
  const existing = tabs.value.find(t => !t.propertiesView && t.sessionID === connection.sessionID && t.node?.name === node.name && t.node?.schema === node.schema && t.node?.database === node.database && !t.disconnected)
  if (existing) { activeTabID.value = existing.id; return }
  newTab(connection)
  const tab = active.value!; tab.node = node; tab.database = node.database; tab.title = node.schema ? `${node.schema}.${node.name}` : node.name
  await previewTab(tab, 0)
}
async function openProperties(source: Tab, event: { sessionID: string; reference: TableReference }) {
  const { sessionID, reference } = event
  const connection = connections.value.find(c => c.sessionID === sessionID)
  if (!connection) return
  const current = () => tabs.value.includes(source) && source.sessionID === sessionID && !source.disconnected
  try {
    const node = await metadata.resolve(sessionID, reference)
    if (!node || !connections.value.includes(connection) || !current()) return
    const existing = tabs.value.find(t => t.propertiesView && t.sessionID === sessionID && t.node?.schema === node.schema && t.node?.name === node.name)
    if (existing) { activeTabID.value = existing.id; await refreshProperties(existing); return }
    newTab(connection)
    const tab = active.value!
    tab.node = node; tab.propertiesView = true; tab.title = `Propriedades: ${node.schema}.${node.name}`
    await refreshProperties(tab)
  } catch (error) { if (connections.value.includes(connection) && current()) announce(errorMessage(error), true) }
}
async function refreshProperties(tab: Tab, invalidate = false) {
  if (!tab.node || tab.disconnected || tab.busy) return
  if (invalidate) invalidateMetadata(tab.sessionID)
  const version = tab.metadataVersion = (tab.metadataVersion || 0) + 1
  tab.busy = true; tab.error = ''
  const current = () => tabs.value.includes(tab) && !tab.disconnected && tab.metadataVersion === version
  try { const properties = await metadata.properties(tab.sessionID, tab.node); if (current() && properties) tab.properties = properties }
  catch (error) { if (current()) tab.error = errorMessage(error) }
  finally { if (current()) tab.busy = false }
}
async function previewTab(tab: Tab, page: number) {
  if (!tab.node || tab.propertiesView || tab.busy || tab.disconnected) return
  tab.busy = true; tab.error = ''
  try {
    const result = await api.Preview(tab.sessionID, { node: tab.node, page })
    if (applyResult(tab, tab.sessionID, result)) {
      tab.page = page
      if (!tab.columns.length) {
        try { tab.columns = await api.Describe(tab.sessionID, tab.node) || [] }
        catch { result.message = [result.message, 'Estrutura indisponível para esta conexão.'].filter(Boolean).join(' ') }
      }
    }
  } catch (e) { if (!tab.disconnected) tab.error = errorMessage(e) }
  finally { tab.busy = false }
}
async function execute(text?: string) {
  const tab = active.value
  if (!tab || tab.busy || tab.disconnected) return
  if (tab.propertiesView) return
  if (tab.node) { await previewTab(tab, tab.page); return }
  if (!text?.trim()) return
  tab.busy = true; tab.error = ''
  try { const result = await api.Execute(tab.sessionID, { text, database: tab.database }); invalidateMetadata(tab.sessionID); applyResult(tab, tab.sessionID, result) }
  catch (e) { if (!tab.disconnected) tab.error = errorMessage(e) }
  finally { tab.busy = false }
}
function selectedChanged() { const c = selected.value; if (c && !tabs.value.some(t => t.sessionID === c.sessionID && !t.disconnected)) newTab(c) }
function keydown(event: KeyboardEvent) {
  const modalOpen = Array.from(document.querySelectorAll('dialog')).some(dialog => (dialog as HTMLDialogElement).open)
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
    event.preventDefault()
    if (!modalOpen && active.value && !active.value.node && !closing.value && !closeRequested.value) void saveScript(active.value)
    return
  }
  if (!modalOpen && event.altKey && event.key.toLowerCase() === 'n' && !dialog.value?.open && !deleteDialog.value?.open && !linkDialog.value?.open) { event.preventDefault(); void openForm() }
}
const editorPercent = ref(43)
function resizeKey(event: KeyboardEvent) {
  if (event.key === 'ArrowUp' || event.key === 'ArrowDown') { event.preventDefault(); editorPercent.value = Math.min(70, Math.max(25, editorPercent.value + (event.key === 'ArrowUp' ? -5 : 5))) }
}
function startResize(event: PointerEvent) {
  const separator = event.currentTarget as HTMLElement, pane = separator.parentElement!
  separator.setPointerCapture(event.pointerId)
  const move = (e: PointerEvent) => { const r = pane.getBoundingClientRect(); editorPercent.value = Math.min(70, Math.max(25, (e.clientY - r.top) / r.height * 100)) }
  const end = () => { separator.removeEventListener('pointermove', move); separator.removeEventListener('lostpointercapture', end); separator.removeEventListener('pointerup', end) }
  separator.addEventListener('pointermove', move); separator.addEventListener('pointerup', end); separator.addEventListener('lostpointercapture', end)
}
onMounted(() => { document.addEventListener('keydown', keydown); if (desktop.value) { unsubscribeClose = onWorkspaceClose(() => { void saveBeforeClose() }); initialization = initialize() } })
onBeforeUnmount(() => { document.removeEventListener('keydown', keydown); unsubscribeClose(); if (workspaceReady.value && !closing.value) void saver.flush().catch(() => {}); saver.dispose() })
</script>

<template>
  <div class="app-shell" :inert="closing || undefined" :aria-busy="closing">
    <aside class="sidebar" aria-label="Conexões e objetos">
      <div class="brand"><span class="brand-mark" aria-hidden="true">◒</span><div>Go DB Viewer<small>EXPLORADOR DE BANCOS</small></div></div>
      <button class="primary new-connection" :disabled="!desktop || globalBusy" @click="openForm()"><span aria-hidden="true">＋</span> Nova conexão <kbd>Alt N</kbd></button>
      <label class="search"><span aria-hidden="true">⌕</span><input v-model="filter" type="search" placeholder="Buscar conexão" aria-label="Buscar conexão salva"></label>
      <div class="sidebar-label">CONEXÕES SALVAS <span>{{ profiles.length }}</span></div>
      <div class="profile-list">
        <div v-if="!profiles.length" class="sidebar-empty">Suas conexões ficam aqui.<br><span>Credenciais no cofre do sistema.</span></div>
        <div v-if="profiles.length && !filteredProfiles.length" class="sidebar-empty">Nenhuma conexão encontrada.</div>
        <article v-for="profile in filteredProfiles" :key="profile.id" class="profile" :class="{ selected: connectionFor(profile)?.sessionID === selectedSession }">
          <button class="profile-open" :disabled="!!starting" @click="connectProfile(profile)"><span class="db-symbol" aria-hidden="true">{{ profile.kind === 'mongodb' ? '{ }' : '▱' }}</span><span class="profile-text">{{ profile.name }}<small>{{ driverName(profile.kind) }}</small></span><span class="connection-state" :class="{ connected: connectionFor(profile) }">{{ starting === profile.id ? '…' : connectionFor(profile) ? 'ON' : 'OFF' }}</span></button>
          <div class="profile-actions"><button class="text-button" @click="openForm(profile)">Editar</button><button class="text-button" @click="requestDelete(profile)">Remover</button></div>
        </article>
      </div>
      <div class="sidebar-label objects-label">OBJETOS <button class="icon-button" title="Atualizar objetos" aria-label="Atualizar objetos" :disabled="!selected || selected.loading" @click="selected && loadRoots(selected)">↻</button></div>
      <div class="objects">
        <template v-if="selected">
          <div v-if="selected.loading" role="status" class="sidebar-empty">Carregando objetos…</div>
          <p v-if="selected.treeError" role="alert" class="tree-error">{{ selected.treeError }}</p>
          <ul v-else class="tree-list" role="tree" aria-label="Objetos do banco"><TreeNode v-for="node in selected.roots" :key="`${selected.sessionID}:${node.name}`" :node="node" :session="selected.sessionID" @open="openNode(selected!, $event)" @database="selectDatabase(selected!, $event)" /></ul>
        </template>
        <div v-else class="sidebar-empty">Conecte um banco para explorar<br>tabelas e coleções.</div>
      </div>
      <div class="sidebar-footer"><span aria-hidden="true">⌘</span><span>Local por padrão<small>Conexão direta com seu banco</small></span></div>
    </aside>

    <main class="workspace">
      <header class="workspace-header"><div class="workspace-heading"><span class="eyebrow">WORKSPACE</span><strong>{{ selected?.name || 'Seus dados, em um só lugar.' }}</strong></div><div class="session-controls"><button class="secondary" :disabled="!workspaceReady" @click="showLibrary()">Abrir script</button><label class="sr-only" for="session">Conexão ativa</label><select id="session" v-model="selectedSession" :disabled="!connections.length" @change="selectedChanged"><option v-if="!connections.length" value="">Sem conexão</option><option v-for="c in connections" :key="c.sessionID" :value="c.sessionID">{{ c.name }}</option></select><button v-if="selected" class="secondary" @click="disconnect(selected)">Desconectar</button></div></header>
      <div v-if="!desktop" class="banner">Prévia da interface. Abra o aplicativo Wails para conectar aos bancos.</div>
      <div v-if="notice" :role="noticeError ? 'alert' : 'status'" class="banner" :class="{ danger: noticeError }"><span>{{ notice }}</span><button class="icon-button" aria-label="Fechar mensagem" @click="notice = ''">×</button></div>

      <div v-if="saveError" role="alert" class="banner danger"><span>{{ saveError }}</span><button class="text-button" @click="retrySaving">Tentar salvar novamente</button></div>
      <template v-if="tabs.length && active">
        <nav class="tab-bar" aria-label="Abas de consulta"><div v-for="tab in tabs" :key="tab.id" class="tab" :class="{ active: tab.id === activeTabID, disconnected: tab.disconnected }"><button :aria-current="tab.id === activeTabID ? 'page' : undefined" @click="activeTabID = tab.id"><span class="tab-language">{{ tab.propertiesView ? 'ⓘ' : tab.kind === 'mongodb' ? '{}' : 'SQL' }}</span>{{ tab.title }}<span v-if="dirty(tab)" aria-label="Alterações não salvas"> ●</span><span v-if="tab.busy" aria-label="Executando">…</span></button><button class="tab-close" :disabled="(tab.busy && !tab.propertiesView) || tab.saving" :aria-label="`Fechar ${tab.title}`" @click="closeTab(tab)">×</button></div><button class="icon-button add-tab" aria-label="Nova consulta" title="Nova consulta" :disabled="!workspaceReady" @click="newTab()">＋</button></nav>
        <div class="query-context"><span class="context-dot" :class="{ offline: active.disconnected }" aria-hidden="true"></span><strong>{{ active.connectionName }}</strong><span>/</span><span>{{ driverName(active.kind) }}</span><template v-if="active.database"><span>/</span><span>{{ active.database }}</span></template><button v-if="active.disconnected && !active.node" class="text-button" @click="showLink(active)">Vincular conexão</button><button v-if="active.disconnected && !active.node" class="secondary" :disabled="!!starting" @click="connectScript(active)">{{ starting ? 'Conectando…' : 'Conectar' }}</button><span class="context-status">{{ active.disconnected ? 'DESCONECTADO' : 'CONECTADO' }}</span></div>
        <PropertiesView v-if="active.propertiesView" :properties="active.properties" :busy="active.busy" :error="active.error" :disconnected="active.disconnected" @refresh="refreshProperties(active!, true)" />
        <section v-else class="query-pane" :class="{ 'preview-pane': active.node }" :style="{ '--editor-size': `${editorPercent}%` }">
          <div v-if="!active.node" class="editor-section">
            <div class="editor-toolbar"><span class="section-label">{{ active.kind === 'mongodb' ? 'COMANDO EXTENDED JSON' : 'EDITOR SQL' }}</span><div class="editor-actions"><span role="status" class="muted">{{ scriptStatus(active) }}</span><button class="secondary" :disabled="!workspaceReady || active.saving || closeRequested" @click="saveScript(active)">Salvar <kbd>Ctrl S</kbd></button><button v-if="active.scriptID" class="text-button" :disabled="active.saving" @click="nameScript({ script: scripts.find(script => script.id === active!.scriptID) })">Renomear</button><label v-if="active.kind === 'mongodb'" class="compact-label">Banco<input v-model="active.database" placeholder="Nome do banco" :disabled="active.busy"></label><button class="primary execute" :disabled="active.busy || active.disconnected || !active.text.trim()" @pointerdown.prevent @click="editor?.executeCurrentCommand()"><span aria-hidden="true">▷</span>{{ active.busy ? 'Executando…' : 'Executar' }}<kbd>⌘ / Ctrl ↵</kbd></button></div></div>
            <CodeEditor ref="editor" :key="active.id" v-model="active.text" :disabled="active.busy || closing" :connected="!active.disconnected" :sessionID="active.sessionID" :anchor="active.anchor" :head="active.head" :load-tables="loadTables" :load-columns="loadColumns" @properties="openProperties(active!, $event)" :dialect="active.kind" :language="active.kind === 'mongodb' ? 'json' : 'sql'" @cursor="Object.assign(active!, $event)" @completion-error="announce(errorMessage($event), true)" @execute="execute" />
            <p v-if="active.scriptError" role="alert" class="script-save-error">{{ active.scriptError }}</p><div class="editor-hint">Separe comandos por linhas vazias. Ctrl+Enter executa o bloco. Ctrl+Espaço sugere nomes. Ctrl+clique abre propriedades.</div>
          </div>
          <div v-if="!active.node" class="resize-handle" role="separator" tabindex="0" aria-label="Redimensionar editor e resultados" aria-orientation="horizontal" :aria-valuenow="editorPercent" aria-valuemin="25" aria-valuemax="70" @pointerdown="startResize" @keydown="resizeKey"><span /></div>
          <section class="results-section" aria-label="Resultados">
            <div class="result-toolbar"><div class="result-heading"><span class="section-label">{{ active.node ? active.title : 'RESULTADOS' }}</span><span v-if="active.result" class="result-count">{{ totalRows }} registros</span></div><div class="result-tools"><template v-if="active.node"><button class="text-button" :disabled="active.busy || active.disconnected || active.page === 0" @click="previewTab(active!, active!.page - 1)">← Anterior</button><span class="muted">Página {{ active.page + 1 }}</span><button class="text-button" :disabled="active.busy || active.disconnected || !active.result?.hasMore" @click="previewTab(active!, active!.page + 1)">Próxima →</button><button class="icon-button" aria-label="Atualizar dados" :disabled="active.busy || active.disconnected" @click="execute()">↻</button></template><span v-if="active.result" class="duration">{{ active.result.elapsedMS }} ms</span></div></div>
            <ResultView :result="active.result" :error="active.error" :busy="active.busy" :columns="active.columns" />
            <div v-if="active.result?.truncated || active.result?.message" class="result-note" role="status">{{ active.result.truncated ? (active.node ? 'Há mais registros ou o limite de dados foi atingido. ' : 'Resultado truncado pelo limite de registros ou tamanho. ') : '' }}{{ active.result.message }}</div>
          </section>
        </section>
      </template>

      <section v-else class="welcome">
        <div class="welcome-overline"><span class="short-rule" />SEUS BANCOS. UM WORKSPACE.</div>
        <h1>Da conexão<br>à consulta.</h1><p class="welcome-description">Explore tabelas e coleções. Execute comandos.<br>Mantenha suas conexões organizadas no mesmo lugar.</p>
        <button class="primary welcome-button" :disabled="!desktop || globalBusy" @click="selected ? newTab() : openForm()">{{ selected ? 'Abrir consulta' : 'Conectar meu primeiro banco' }}<span aria-hidden="true">↗</span></button>
        <div class="welcome-steps"><div><span class="step-number">01</span><strong>Escolha o banco</strong><p>SQL ou documentos.</p></div><div><span class="step-number">02</span><strong>Informe a conexão</strong><p>Uma string completa.</p></div><div><span class="step-number">03</span><strong>Explore seus dados</strong><p>Editor e resultados integrados.</p></div></div>
        <div class="supported-databases"><span>PostgreSQL</span><span>Oracle</span><span>SQL Server</span><span>MongoDB</span><span>Ignite 2 / 3</span></div>
      </section>
      <footer class="status-bar"><span>{{ connections.length }} conexão(ões) ativa(s) <span aria-hidden="true">·</span> <span role="status" :class="{ danger: !!saveError }">Abas: {{ saveStatus }}</span></span><span>Credenciais no cofre do sistema</span><span>Go DB Viewer <span class="muted">0.1</span></span></footer>
    </main>

    <dialog ref="libraryDialog" class="connection-dialog script-library-dialog" aria-labelledby="script-library-title" @cancel.prevent="closeLibrary">
      <ScriptLibrary :key="libraryVersion" :scripts="scripts" :profiles="profiles" :drivers="drivers" :profileID="libraryProfileID" :locked="!workspaceReady || closing || closeRequested" @close="closeLibrary" @create="createFromLibrary" @open="openSavedScript" @rename="nameScript({ script: $event })" @remove="requestRemoveScript" />
    </dialog>
    <dialog ref="nameDialog" class="delete-dialog" aria-labelledby="script-name-title" @cancel.prevent="closeName()">
      <form @submit.prevent="submitName"><h2 id="script-name-title">{{ naming?.script ? 'Renomear script' : 'Salvar script' }}</h2><label for="script-name">Nome do script</label><input id="script-name" v-model="nameValue" autofocus :disabled="nameBusy" aria-describedby="script-name-error"><p id="script-name-error" v-if="nameError" class="danger" role="alert">{{ nameError }}</p><div class="delete-actions"><button type="button" class="secondary" :disabled="nameBusy" @click="closeName()">Cancelar</button><button type="submit" class="primary" :disabled="nameBusy">{{ nameBusy ? 'Salvando…' : naming?.script ? 'Renomear' : 'Salvar' }}</button></div></form>
    </dialog>
    <dialog ref="changesDialog" class="delete-dialog" aria-labelledby="script-changes-title" @cancel.prevent="decideChanges('cancel')">
      <h2 id="script-changes-title">Salvar alterações?</h2><p>O script “{{ changesTab?.title }}” tem alterações não salvas.</p><p v-if="changesError" class="danger" role="alert">{{ changesError }}</p><div class="delete-actions"><button class="text-button" :disabled="changesBusy" @click="decideChanges('discard')">Descartar alterações</button><button class="secondary" :disabled="changesBusy" @click="decideChanges('cancel')">Cancelar</button><button class="primary" :disabled="changesBusy" @click="decideChanges('save')">Salvar</button></div>
    </dialog>
    <dialog ref="removeScriptDialog" class="delete-dialog" aria-labelledby="script-remove-title" @cancel.prevent="cancelRemoveScript">
      <h2 id="script-remove-title">Excluir script?</h2><p>O script “{{ removingScript?.title }}” será excluído. A aba correspondente será fechada, incluindo alterações não salvas.</p><p v-if="removeScriptError" class="danger" role="alert">{{ removeScriptError }}</p><div class="delete-actions"><button class="secondary" :disabled="removeScriptBusy" @click="cancelRemoveScript">Cancelar</button><button class="primary" :disabled="removeScriptBusy" @click="removeScript">Excluir script</button></div>
    </dialog>
    <dialog ref="dialog" :inert="closing || undefined" class="connection-dialog" aria-labelledby="connection-title" @cancel.prevent="closeForm">
      <form @submit.prevent="connectForm">
        <header class="dialog-header"><div><span class="eyebrow">CONEXÃO</span><h2 id="connection-title">{{ form.id ? 'Editar conexão' : 'Nova conexão' }}</h2></div><button type="button" class="icon-button" aria-label="Fechar formulário" :disabled="formBusy" @click="closeForm">×</button></header>
        <div class="dialog-body"><div class="form-row"><label>Nome da conexão<input v-model="form.name" autofocus placeholder="Ex.: Desenvolvimento" maxlength="120" :disabled="formBusy"></label><label>Tipo do banco<select v-model="form.kind" :disabled="formBusy" @change="formStatus = ''"><option v-for="driver in drivers" :key="driver.kind" :value="driver.kind">{{ driver.name }}</option></select></label></div>
          <label for="dsn">String de conexão completa</label><div class="dsn-input"><input id="dsn" v-model="form.dsn" :type="reveal ? 'text' : 'password'" :placeholder="form.id ? 'Deixe vazio para usar a string do cofre' : 'Cole a string com usuário e senha'" autocomplete="off" autocapitalize="off" spellcheck="false" :disabled="formBusy" :aria-invalid="!!formError" aria-describedby="dsn-help"><button type="button" class="text-button" :disabled="formBusy" :aria-pressed="reveal" @click="reveal = !reveal">{{ reveal ? 'Ocultar' : 'Mostrar' }}</button></div>
          <p id="dsn-help" class="field-help">Inclua usuário, senha e opções do driver. Codifique caracteres especiais conforme o formato escolhido.</p><div class="dsn-example"><span>EXEMPLO DE FORMATO</span><code>{{ formDriver?.example }}</code></div>
          <p v-if="form.kind.startsWith('ignite')" class="field-help">Ignite exige o driver ODBC oficial. No macOS, JDBC também permite conectar pelo processo Java auxiliar.</p>
          <p v-if="formError" class="form-message danger" role="alert">{{ formError }}</p><p v-if="formStatus" class="form-message success" role="status">{{ formStatus }}</p>
          <div class="credential-note"><span aria-hidden="true">◇</span><p><strong>Credenciais no cofre do sistema</strong><br>Salvar mantém a string no cofre. Conectar sem salvar mantém a string apenas nesta sessão.</p></div>
        </div>
        <footer class="dialog-footer"><button type="button" class="text-button" :disabled="formBusy" @click="testForm">Testar conexão</button><div><button type="button" class="secondary" :disabled="formBusy || !form.name.trim()" @click="saveForm">Salvar no cofre</button><button type="submit" class="primary" :disabled="formBusy">{{ formBusy ? 'Aguarde…' : form.id ? 'Conectar' : 'Conectar sem salvar' }}</button></div></footer>
      </form>
    </dialog>
    <dialog ref="linkDialog" :inert="closing || undefined" class="delete-dialog" aria-labelledby="link-title" @cancel.prevent="closeLink">
      <h2 id="link-title">Vincular conexão</h2>
      <p>Escolha uma conexão de {{ driverName(tabs.find(tab => tab.id === linkTabID)?.kind || 'postgresql') }} para esta aba.</p>
      <label for="link-connection">Conexão</label>
      <select id="link-connection" v-model="linkValue" :disabled="linkBusy">
        <option value="">Escolha uma conexão</option>
        <optgroup label="Conexões ativas"><option v-for="c in connections.filter(c => c.kind === tabs.find(tab => tab.id === linkTabID)?.kind)" :key="c.sessionID" :value="`active:${c.sessionID}`">{{ c.name }}</option></optgroup>
        <optgroup label="Perfis salvos"><option v-for="p in profiles.filter(p => p.kind === tabs.find(tab => tab.id === linkTabID)?.kind)" :key="p.id" :value="`profile:${p.id}`">{{ p.name }}</option></optgroup>
      </select>
      <p v-if="linkError" class="danger" role="alert">{{ linkError }}</p>
      <p>Para criar outra conexão, feche esta janela e use Nova conexão.</p>
      <div class="delete-actions"><button class="secondary" :disabled="linkBusy" @click="closeLink">Cancelar</button><button class="primary" :disabled="linkBusy || !linkValue" @click="linkSelected">{{ linkBusy ? 'Conectando…' : 'Vincular' }}</button></div>
    </dialog>
    <dialog ref="deleteDialog" :inert="closing || undefined" class="delete-dialog" aria-labelledby="delete-title" @cancel.prevent="closeDelete"><h2 id="delete-title">Remover conexão?</h2><p>A conexão “{{ pendingDelete?.name }}” e sua credencial serão removidas.</p><div class="delete-actions"><button class="secondary" :disabled="globalBusy" autofocus @click="closeDelete">Cancelar</button><button class="primary" :disabled="globalBusy" @click="removeProfile">Remover conexão</button></div></dialog>
  </div>
</template>
