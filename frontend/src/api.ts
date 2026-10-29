export type Kind = 'postgresql' | 'oracle' | 'sqlserver' | 'mongodb' | 'ignite2' | 'ignite3'
export interface DriverInfo { kind: Kind; name: string; example: string; language: string }
export interface Profile { id: string; name: string; kind: Kind; hasSecret: boolean }
export interface ConnectionInput { id: string; name: string; kind: Kind; dsn: string }
export interface DBNode { name: string; kind: string; database: string; schema: string }
export interface Column { name: string; type: string; insertText?: string }
export interface Cell { text: string; null: boolean; truncated: boolean }
export interface ResultSet { columns: Column[]; rows: Cell[][] }
export interface Settings { theme: 'system' | 'light' | 'dark'; maxRows: number }
export interface SettingsLoad { settings: Settings; warning: string }
export interface Result { rowLimit: number; limitReason: string; sets: ResultSet[]; documents: string[]; affected: string; elapsedMS: number; truncated: boolean; hasMore: boolean; message: string }
export interface Query { rowLimit?: number; database: string; text: string }
export interface TableSuggestion { schema: string; name: string; insertText: string }
export interface ScriptTab { scriptID?: string; initialText?: string; initialDatabase?: string; id: number; title: string; text: string; kind: Kind; profileID: string; connectionName: string; database: string; anchor: number; head: number }
export interface SavedScript { id: string; title: string; text: string; kind: Kind; profileID: string; connectionName: string; database: string; updatedAt: string }
export type ScriptInput = Omit<SavedScript, 'updatedAt'>
export interface OpenScript { id: number; scriptID: string; anchor: number; head: number }
export interface WorkspaceState { version: number; activeTabID: number; openTabs: OpenScript[] }
export interface Workspace extends WorkspaceState { scripts: SavedScript[] }
export interface WorkspaceLoad { workspace: Workspace; warning: string }
export interface TableReference { schema: string; name: string; schemaQuoted: boolean; nameQuoted: boolean }
export interface MetadataSection { status: 'available' | 'unavailable' | 'unsupported'; message: string; columns: string[]; rows: Cell[][] }
export interface TableProperties { table: DBNode; columns: MetadataSection; primaryKey: MetadataSection; foreignKeys: MetadataSection; indexes: MetadataSection }
interface Backend {
  LoadSettings(): Promise<SettingsLoad>
  SaveSettings(settings: Settings): Promise<Settings>
  ResolveTable(id: string, reference: TableReference): Promise<DBNode>
  GetTableProperties(id: string, node: DBNode): Promise<TableProperties>
  ListTables(id: string): Promise<TableSuggestion[]>
  SaveScript(script: ScriptInput): Promise<SavedScript>
  RenameScript(id: string, title: string): Promise<SavedScript>
  DeleteScript(id: string): Promise<void>
  LoadWorkspace(): Promise<WorkspaceLoad>
  SaveWorkspace(workspace: WorkspaceState): Promise<void>
  SaveWorkspaceAndClose(workspace: WorkspaceState): Promise<void>
  Drivers(): Promise<DriverInfo[]>
  Profiles(): Promise<Profile[]>
  SaveProfile(input: ConnectionInput): Promise<Profile>
  DeleteProfile(id: string): Promise<void>
  TestConnection(input: ConnectionInput): Promise<void>
  Connect(input: ConnectionInput): Promise<string>
  Disconnect(id: string): Promise<void>
  List(id: string, node: DBNode): Promise<DBNode[]>
  Describe(id: string, node: DBNode): Promise<Column[]>
  Preview(id: string, req: { node: DBNode; page: number; rowLimit?: number }): Promise<Result>
  Execute(id: string, query: Query): Promise<Result>
}
declare global { interface Window { go?: { main?: { App?: Backend } }; runtime?: { EventsOnMultiple(name: string, callback: () => void, count: number): () => void } } }
export function onWorkspaceClose(callback: () => void): () => void {
  return window.runtime?.EventsOnMultiple('workspace:save-before-close', callback, -1) || (() => {})
}
export const isDesktop = () => Boolean(window.go?.main?.App)
// Os métodos mantêm o contrato dos bindings gerados pelo Wails.
export const api = new Proxy({} as Backend, {
  get(_target, method: keyof Backend) {
    return (...args: unknown[]) => {
      const backend = window.go?.main?.App
      if (!backend) return Promise.reject(new Error('Abra o aplicativo desktop para acessar os bancos.'))
      const fn = backend[method] as (...args: unknown[]) => Promise<unknown>
      return fn(...args)
    }
  },
})
export const rootNode = (): DBNode => ({ name: '', kind: '', database: '', schema: '' })
export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
