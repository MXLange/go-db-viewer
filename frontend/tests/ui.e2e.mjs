import { chromium } from 'playwright-core'
import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || undefined, headless: true, args: ['--no-sandbox', '--allow-file-access-from-files'] })
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 } })
  const failures = []
  let savedWorkspace = { version: 2, activeTabID: 0, scripts: [], openTabs: [] }
  let savedSettings = {theme:'system',maxRows:200}
  await page.exposeFunction('loadSettingsForTest',()=>structuredClone(savedSettings))
  await page.exposeFunction('saveSettingsForTest',settings=>{savedSettings=structuredClone(settings);return savedSettings})
  let scriptSequence = 0
  await page.exposeFunction('loadWorkspaceForTest', () => structuredClone(savedWorkspace))
  await page.exposeFunction('saveWorkspaceForTest', state => {
    assert.equal('scripts' in state, false)
    savedWorkspace = { ...savedWorkspace, ...structuredClone(state) }
  })
  await page.exposeFunction('saveScriptForTest', script => {
    const title = script.title.trim()
    if (savedWorkspace.scripts.some(other => other.id !== script.id && other.profileID === script.profileID && other.title.toLowerCase() === title.toLowerCase())) throw new Error('Já existe um script com este nome nesta conexão.')
    const saved = { ...script, title, id: script.id || `script-${++scriptSequence}`, updatedAt: new Date().toISOString() }
    const index = savedWorkspace.scripts.findIndex(other => other.id === saved.id)
    if (index < 0) savedWorkspace.scripts.push(structuredClone(saved))
    else savedWorkspace.scripts[index] = structuredClone(saved)
    return saved
  })
  await page.exposeFunction('renameScriptForTest', ({id,title}) => {
    const script = savedWorkspace.scripts.find(script => script.id === id)
    script.title = title; script.updatedAt = new Date().toISOString()
    return structuredClone(script)
  })
  await page.exposeFunction('deleteScriptForTest', id => {
    savedWorkspace.scripts = savedWorkspace.scripts.filter(script => script.id !== id)
    savedWorkspace.openTabs = savedWorkspace.openTabs.filter(tab => tab.scriptID !== id)
  })
  page.on('pageerror', error => failures.push(error.message))
  await page.addInitScript(() => {
    const profiles = [], sessions = new Map()
    let sequence = 0
    window.executedQueries = []
    window.connectionAttempts = 0
    window.catalogCalls = 0
    window.propertyCalls = 0
    window.previewCalls = 0
    window.runtime = { EventsOnMultiple: (_name, callback) => { window.closeWorkspace = callback; return () => {} } }
    const result = { rowLimit:200, limitReason:'', sets: [{ columns: [{ name: 'id', type: 'BIGINT' }], rows: [[{ text: '9223372036854775807', null: false, truncated: false }]] }], documents: [], affected: '', elapsedMS: 4, truncated: false, message: '' }
    window.go = { main: { App: {
      LoadSettings: async()=>({settings:await window.loadSettingsForTest(),warning:''}),
      SaveSettings: async settings=>{if(window.failSettingsSave)throw new Error('Falha controlada nas configurações.');const result=await window.saveSettingsForTest(settings);window.lastSettings=result;return result},
      SaveScript: async script => { if (window.failScriptSave) throw new Error('Falha controlada ao salvar.'); return window.saveScriptForTest(script) },
      RenameScript: async (id,title) => window.renameScriptForTest({id,title}),
      DeleteScript: async id => window.deleteScriptForTest(id),
      LoadWorkspace: async () => ({ workspace: await window.loadWorkspaceForTest(), warning: '' }),
      SaveWorkspace: async workspace => window.saveWorkspaceForTest(workspace),
      SaveWorkspaceAndClose: async workspace => { await window.saveWorkspaceForTest(workspace); window.workspaceClosed = true },
      ListTables: async () => { window.catalogCalls++; return [
        { schema: 'public', name: 'clientes', insertText: '"public"."clientes"' },
        { schema: 'sales', name: 'clientes', insertText: '"sales"."clientes"' },
      ] },
      Drivers: async () => ['postgresql', 'oracle', 'sqlserver', 'mongodb', 'ignite2', 'ignite3'].map(kind => ({ kind, name: kind, example: `${kind}://usuario:senha@localhost/banco`, language: kind === 'mongodb' ? 'json' : 'sql' })),
      Profiles: async () => [...profiles],
      SaveProfile: async input => { const profile = { id: 'saved', name: input.name, kind: input.kind, hasSecret: true }; profiles.push(profile); return profile },
      DeleteProfile: async id => { profiles.splice(profiles.findIndex(p => p.id === id), 1) },
      TestConnection: async () => {},
      Connect: async input => { window.connectionAttempts++; const id = String(++sequence); sessions.set(id, input.kind); return id },
      Disconnect: async id => { sessions.delete(id) },
      List: async (id, parent) => {
        if (!parent.kind) return [{ name: 'banco', kind: 'database', database: 'banco', schema: '' }]
        if (parent.kind === 'database') return [{ name: 'public', kind: 'schema', database: 'banco', schema: 'public' }]
        return [{ name: 'clientes', kind: 'table', database: 'banco', schema: 'public' }]
      },
      ResolveTable: async (_id, ref) => ({name:ref.name,schema:ref.schema || 'public',database:'banco',kind:'table'}),
      GetTableProperties: async (_id,node) => {
        window.propertyCalls++
        const cell=text=>({text,null:false,truncated:false})
        const empty={status:'available',message:'',columns:[],rows:[]}
        return {table:node,columns:{...empty,columns:['Coluna','Tipo'],rows:[[cell('id'),cell('BIGINT')]]},primaryKey:empty,foreignKeys:{...empty,status:'unsupported',message:'O driver não expõe estes metadados.'},indexes:empty}
      },
      Describe: async () => [{ name: 'id', type: 'BIGINT', insertText:'"id"' }, {name:'nome',type:'TEXT',insertText:'"nome"'}],
      Preview: async (_id,req) => { window.previewCalls++;window.lastPreview=req;return {...structuredClone(result),rowLimit:req.rowLimit||200} },
      Execute: async (_id, query) => {
        window.lastQuery = query
        window.executedQueries.push(query)
        if (query.text.includes('falha')) throw new Error('Falha controlada de execução.')
        if(query.text==='SELECT many') {
          const limit=query.rowLimit||200
          return {...structuredClone(result),rowLimit:limit,truncated:true,hasMore:true,limitReason:'rows',sets:[{columns:[{name:'id',type:'INT'}],rows:Array.from({length:limit},(_,i)=>[{text:String(i+1),null:false,truncated:false}])}]}
        }
        return /^INSERT/i.test(query.text) ? { ...result, rowLimit:0, sets: [], affected: '7', message: 'Comando concluído.' } : {...structuredClone(result),rowLimit:query.rowLimit||200}
      },
    } } }
  })
  const url = new URL('../dist/index.html', import.meta.url).href
  await page.goto(url)
  await page.getByRole('heading', { name: 'Da conexão à consulta.' }).waitFor()
  await page.getByRole('button', { name: 'Nova conexão' }).click()
  await page.getByRole('textbox', { name: 'Nome da conexão' }).fill('Desenvolvimento')
  await page.locator('#dsn').fill('postgresql://user:segredo@localhost/db')
  assert.equal(await page.locator('#dsn').getAttribute('type'), 'password')
  await page.getByRole('button', { name: 'Testar conexão', exact: true }).click()
  await page.getByText('Conexão testada com sucesso.').waitFor()
  await page.getByRole('button', { name: 'Salvar no cofre' }).click()
  await page.getByText('Conexão salva. A string está no cofre do sistema.').waitFor()
  assert.equal(await page.locator('#dsn').inputValue(), '')
  await page.getByRole('button',{name:'Fechar formulário',exact:true}).click()
  await page.locator('.profile-open').click()
  await page.getByRole('heading',{name:'Scripts salvos'}).waitFor()
  assert.equal(await page.evaluate(()=>window.connectionAttempts),0)
  await page.getByRole('button',{name:'Novo script'}).click()
  await page.locator('.code-editor .cm-content').waitFor()
  assert.equal(await page.getByRole('button',{name:'Executar'}).isDisabled(),true)
  await page.getByRole('button',{name:'Fechar Nova consulta',exact:true}).click()
  await page.getByRole('button',{name:'Editar',exact:true}).click()
  await page.getByRole('button', { name: 'Conectar', exact: true }).click()
  const editor = page.locator('.code-editor .cm-content')
  const modifier = process.platform === 'darwin' ? 'Meta' : 'Control'
  await editor.waitFor()
  await page.getByRole('button', {name:'Fechar Nova consulta',exact:true}).click()
  const attemptsBeforeLibrary = await page.evaluate(() => window.connectionAttempts)
  await page.locator('.profile-open').click()
  await page.getByRole('heading',{name:'Scripts salvos'}).waitFor()
  assert.equal(await page.evaluate(() => window.connectionAttempts), attemptsBeforeLibrary)
  await page.getByRole('button',{name:'Novo script'}).click()
  await editor.waitFor()
  await editor.click()
  await editor.press(`${modifier}+a`)
  await page.keyboard.insertText('SELECT * FROM ')
  await editor.press('Control+Space')
  await page.locator('.cm-tooltip-autocomplete [role="option"]').first().waitFor()
  // O CodeMirror bloqueia a aceitação nos primeiros 75 ms da lista.
  await page.waitForTimeout(100)
  assert.equal(await page.locator('.cm-tooltip-autocomplete [role="option"]').count(), 2)
  await editor.press('Enter')
  assert.equal((await editor.locator('.cm-line').allTextContents()).join('\n'), 'SELECT * FROM "public"."clientes"')
  await editor.press(`${modifier}+a`)
  await page.keyboard.insertText('SELECT * FROM sales.cl')
  await editor.press('Control+Space')
  await page.locator('.cm-tooltip-autocomplete [role="option"]').first().waitFor()
  // O CodeMirror bloqueia a aceitação nos primeiros 75 ms da lista.
  await page.waitForTimeout(100)
  await editor.press('Tab')
  assert.equal((await editor.locator('.cm-line').allTextContents()).join('\n'), 'SELECT * FROM "sales"."clientes"')
  assert.equal(await page.evaluate(() => window.catalogCalls), 1)
  assert.equal(await page.getByRole('combobox', { name: 'Modo de execução' }).count(), 0)
  await editor.press(`${modifier}+a`)
  await page.keyboard.insertText('SELECT * FROM "public"."clientes" C\nWHERE C')
  await editor.press('.')
  await page.locator('.cm-tooltip-autocomplete [role="option"]').first().waitFor()
  // O CodeMirror bloqueia a aceitação nos primeiros 75 ms da lista.
  await page.waitForTimeout(100)
  assert.equal(await page.locator('.cm-tooltip-autocomplete [role="option"]').count(),2)
  await editor.press('Tab')
  assert.ok((await editor.locator('.cm-line').allTextContents()).join('\n').endsWith('C."id"'))
  await editor.press(`${modifier}+a`)
  await page.keyboard.insertText('SELECT * FROM "public"."clientes" C\nWHERE ')
  await editor.press('Control+Space')
  await page.locator('.cm-tooltip-autocomplete [role="option"]').first().waitFor()
  // O CodeMirror bloqueia a aceitação nos primeiros 75 ms da lista.
  await page.waitForTimeout(100)
  await editor.press('Escape')
  const point = await editor.locator('.cm-line').first().evaluate(element => {
    const walker=document.createTreeWalker(element,NodeFilter.SHOW_TEXT)
    while(walker.nextNode()) {
      const node=walker.currentNode, index=node.textContent.indexOf('clientes')
      if(index<0)continue
      const range=document.createRange();range.setStart(node,index+2);range.setEnd(node,index+3)
      const rect=range.getBoundingClientRect();return {x:rect.x+rect.width/2,y:rect.y+rect.height/2}
    }
  })
  await page.keyboard.down('Control');await page.mouse.click(point.x,point.y);await page.keyboard.up('Control')
  await page.getByRole('region',{name:'Propriedades da tabela'}).waitFor()
  await page.getByRole('heading',{name:'Colunas',exact:true}).waitFor()
  assert.equal(await page.getByRole('button',{name:'Executar'}).count(),0)
  assert.equal(await page.evaluate(()=>window.executedQueries.length),0)
  assert.equal(await page.evaluate(()=>window.previewCalls),0)
  await page.getByRole('button',{name:'Atualizar propriedades'}).click()
  await page.waitForFunction(()=>window.propertyCalls===2)
  await page.locator('.tab').filter({hasText:'Nova consulta'}).locator('button').first().click()
  await editor.click()
  await page.keyboard.down('Control');await page.mouse.click(point.x,point.y);await page.keyboard.up('Control')
  await page.getByRole('region',{name:'Propriedades da tabela'}).waitFor()
  assert.equal(await page.locator('.tab').count(),2)
  await page.locator('.tab').filter({hasText:'Nova consulta'}).locator('button').first().click()

  const document = "SELECT 1;\n\nINSERT INTO clientes (id) VALUES (7);\n\nSELECT 'falha';"
  await editor.click()
  await editor.press(`${modifier}+a`)
  await page.keyboard.insertText(document)
  await editor.press('Control+Home')
  await editor.press(`${modifier}+Enter`)
  await page.getByRole('cell', { name: '9223372036854775807', exact: true }).waitFor()
  assert.equal(await editor.evaluate(element => element === window.document.activeElement), true)
  assert.equal(await page.evaluate(() => window.lastQuery.text), 'SELECT 1;')
  assert.equal((await editor.locator('.cm-line').allTextContents()).join('\n'), document)
  await editor.press('ArrowDown')
  await editor.press(`${modifier}+Enter`)
  assert.equal(await page.evaluate(() => window.executedQueries.length), 1)
  await editor.press('ArrowDown')
  await editor.press(`${modifier}+Enter`)
  await page.getByRole('heading', { name: '7 linha(s) afetada(s)' }).waitFor()
  assert.equal(await page.evaluate(() => window.lastQuery.text), 'INSERT INTO clientes (id) VALUES (7);')
  assert.equal(await page.evaluate(() => 'mode' in window.lastQuery), false)
  await page.getByRole('button', { name: 'Executar' }).click()
  await page.waitForFunction(() => window.executedQueries.length === 3)
  await page.getByRole('combobox', { name: 'Conexão ativa' }).focus()
  await page.keyboard.press(`${modifier}+Enter`)
  await page.getByRole('button', { name: 'Executar' }).click()
  assert.equal(await page.evaluate(() => window.executedQueries.length), 3)
  await editor.focus()
  await editor.press('Control+Home')
  await editor.press('Control+Shift+End')
  await editor.press(`${modifier}+Enter`)
  await page.getByText('Falha controlada de execução.', { exact: true }).waitFor()
  assert.equal(await page.evaluate(() => window.lastQuery.text), "SELECT 'falha';")
  await editor.press(`${modifier}+a`)
  await page.keyboard.insertText('-- somente comentário')
  await editor.press(`${modifier}+Enter`)
  assert.equal(await page.evaluate(() => window.executedQueries.length), 4)
  await page.getByRole('treeitem', { name: 'banco' }).click()
  await page.getByRole('treeitem', { name: 'public schema' }).click()
  await page.getByRole('treeitem', { name: 'clientes' }).click()
  await page.getByRole('button', { name: 'Ver estrutura' }).click()
  await page.getByRole('cell', { name: 'BIGINT', exact: true }).waitFor()
  await page.getByRole('button', { name: 'Desconectar', exact: true }).click()
  await page.getByText('DESCONECTADO', { exact: true }).waitFor()
  assert.equal(await page.getByRole('button', { name: 'Atualizar dados' }).isDisabled(), true)
  await page.getByRole('button', { name: 'Remover', exact: true }).click()
  await page.getByRole('button', { name: 'Remover conexão', exact: true }).click()
  await page.getByText('Conexão e credencial removidas.').waitFor()
  await page.locator('.tab').filter({ hasText: 'Nova consulta' }).locator('button').first().click()
  await editor.click()
  await editor.press(`${modifier}+a`)
  const persistedText = "SELECT 'restaurado';"
  await page.keyboard.insertText(persistedText)
  await editor.press('ArrowLeft')
  await editor.press(`${modifier}+Enter`)
  assert.equal(await page.evaluate(() => window.executedQueries.length), 4)
  // Alterar o texto não grava a biblioteca.
  assert.equal(savedWorkspace.scripts.length,0)
  await editor.press(`${modifier}+s`)
  const nameDialog=page.getByRole('dialog',{name:'Salvar script',exact:true})
  await nameDialog.waitFor()
  await nameDialog.getByRole('textbox',{name:'Nome do script'}).fill('Relatório clientes')
  await page.evaluate(()=>window.failScriptSave=true)
  await nameDialog.getByRole('button',{name:'Salvar',exact:true}).click()
  await page.getByText('Falha controlada ao salvar.',{exact:true}).first().waitFor()
  assert.equal(savedWorkspace.scripts.length,0)
  await page.evaluate(()=>window.failScriptSave=false)
  await nameDialog.getByRole('button',{name:'Salvar',exact:true}).click()
  await nameDialog.waitFor({state:'hidden'})
  assert.equal(savedWorkspace.scripts[0].text,persistedText)
  await editor.focus();await editor.press('Control+End');await page.keyboard.insertText(' -- pendente')
  await page.getByRole('button',{name:'Renomear',exact:true}).click()
  const renameDialog=page.getByRole('dialog',{name:'Renomear script',exact:true})
  await renameDialog.getByRole('textbox',{name:'Nome do script'}).fill('Clientes salvos')
  await renameDialog.getByRole('button',{name:'Renomear',exact:true}).click()
  await renameDialog.waitFor({state:'hidden'})
  assert.equal(savedWorkspace.scripts[0].text,persistedText)
  await page.getByRole('button',{name:'Fechar Clientes salvos',exact:true}).click()
  const changesDialog=page.getByRole('dialog',{name:'Salvar alterações?',exact:true})
  await changesDialog.waitFor()
  await changesDialog.getByRole('button',{name:'Cancelar',exact:true}).click()
  await editor.waitFor()
  assert.ok((await editor.locator('.cm-line').allTextContents()).join('\n').endsWith(' -- pendente'))
  await page.getByRole('button',{name:'Fechar Clientes salvos',exact:true}).click()
  await changesDialog.getByRole('button',{name:'Descartar alterações',exact:true}).click()
  assert.equal(savedWorkspace.scripts.length,1)
  await page.getByRole('button',{name:'Abrir script',exact:true}).click()
  const libraryDialog=page.getByRole('dialog',{name:'Scripts salvos',exact:true})
  await libraryDialog.getByRole('combobox',{name:'Conexão dos scripts'}).selectOption('$unlinked')
  await libraryDialog.getByRole('button',{name:'Clientes salvos',exact:false}).first().click()
  await editor.waitFor()
  assert.equal((await editor.locator('.cm-line').allTextContents()).join('\n'),persistedText)
  await editor.focus();await editor.press('Control+End');await page.keyboard.insertText(' -- sair')
  await page.getByRole('button',{name:'Nova consulta',exact:true}).click()
  await libraryDialog.waitFor()
  await libraryDialog.getByRole('button',{name:'Novo script'}).click()
  await editor.waitFor();await editor.focus();await editor.press(`${modifier}+a`);await page.keyboard.insertText('SELECT 2;')
  await page.evaluate(() => window.closeWorkspace())
  await changesDialog.waitFor()
  await changesDialog.getByRole('button',{name:'Descartar alterações',exact:true}).click()
  await changesDialog.getByText('O script “Nova consulta” tem alterações não salvas.').waitFor()
  await changesDialog.getByRole('button',{name:'Cancelar',exact:true}).click()
  assert.equal(await page.evaluate(()=>window.workspaceClosed),undefined)
  await page.evaluate(() => window.closeWorkspace())
  await changesDialog.getByRole('button',{name:'Descartar alterações',exact:true}).click()
  await changesDialog.getByText('O script “Nova consulta” tem alterações não salvas.').waitFor()
  await changesDialog.getByRole('button',{name:'Descartar alterações',exact:true}).click()
  await page.waitForFunction(() => window.workspaceClosed === true)
  assert.equal(savedWorkspace.scripts.length,1)
  assert.equal(savedWorkspace.scripts[0].text,persistedText)
  assert.equal(savedWorkspace.openTabs.length,1)
  assert.equal('sessionID' in savedWorkspace.scripts[0],false)
  await page.reload()
  await editor.waitFor()
  assert.equal((await editor.locator('.cm-line').allTextContents()).join('\n'),persistedText)
  assert.equal(await page.evaluate(() => window.connectionAttempts),0)
  assert.equal(await page.locator('.tab').count(),1)
  assert.equal(await editor.getAttribute('aria-readonly'),'false')
  await page.getByRole('button',{name:'Vincular conexão',exact:true}).waitFor()
  // Limites por aba, configurações e temas não executam comandos automaticamente.
  await page.getByRole('button',{name:'Nova conexão'}).click()
  await page.getByRole('textbox',{name:'Nome da conexão'}).fill('Teste de limites')
  await page.locator('#dsn').fill('postgresql://user:secret@localhost/db')
  await page.getByRole('button',{name:'Conectar sem salvar',exact:true}).click()
  await editor.waitFor();await editor.focus();await editor.press(`${modifier}+a`);await page.keyboard.insertText('SELECT many')
  await editor.press(`${modifier}+Enter`)
  await page.getByText('200/200 máx',{exact:true}).waitFor()
  const limitInput=page.getByRole('textbox',{name:'Máximo nesta aba',exact:true})
  await limitInput.fill('/1000');await limitInput.press('Enter')
  assert.equal(await page.evaluate(()=>window.executedQueries.length),1)
  await page.getByText('Novo limite na próxima execução.',{exact:true}).waitFor()
  await page.getByRole('button',{name:'Configurações',exact:true}).click()
  const settingsView=page.getByRole('region',{name:'Configurações',exact:true})
  const themeSelect=settingsView.getByRole('combobox',{name:'Tema',exact:true})
  await themeSelect.selectOption('light')
  await page.waitForFunction(()=>document.documentElement.dataset.theme==='light'&&window.lastSettings?.theme==='light')
  const defaultLimit=settingsView.getByRole('textbox',{name:'Máximo de linhas/itens',exact:true})
  await defaultLimit.fill('0');await defaultLimit.press('Enter')
  await settingsView.getByText('Informe um limite inteiro entre 1 e 100.000.',{exact:true}).waitFor()
  assert.equal(savedSettings.maxRows,200)
  await page.evaluate(()=>window.failSettingsSave=true)
  await defaultLimit.fill('500');await defaultLimit.press('Enter')
  await settingsView.getByText('Falha controlada nas configurações.',{exact:true}).waitFor()
  assert.equal(savedSettings.maxRows,200)
  await page.evaluate(()=>window.failSettingsSave=false)
  await settingsView.getByRole('button',{name:'Tentar novamente',exact:true}).click()
  await page.waitForFunction(()=>window.lastSettings?.maxRows===500)
  await page.getByRole('button',{name:'Workspace',exact:true}).click()
  assert.equal((await editor.locator('.cm-line').allTextContents()).join('\n'),'SELECT many')
  assert.equal(await limitInput.inputValue(),'1000')
  assert.equal(await page.evaluate(()=>window.executedQueries.length),1)
  await editor.focus();await editor.press(`${modifier}+Enter`)
  await page.getByText('1000/1000 máx',{exact:true}).waitFor()
  assert.equal(await page.evaluate(()=>window.lastQuery.rowLimit),1000)
  await editor.press(`${modifier}+a`)
  await page.locator('.cm-selectionBackground').first().waitFor()
  const lightSelection=await page.locator('.cm-selectionBackground').first().evaluate(el=>getComputedStyle(el).backgroundColor)
  assert.notEqual(lightSelection,'rgb(255, 255, 255)')
  await page.getByRole('button',{name:'Configurações',exact:true}).click()
  await page.emulateMedia({colorScheme:'dark'})
  assert.equal(await page.evaluate(()=>document.documentElement.dataset.theme),'light')
  await themeSelect.selectOption('system')
  await page.waitForFunction(()=>document.documentElement.dataset.theme==='dark')
  await page.emulateMedia({colorScheme:'light'})
  await page.waitForFunction(()=>document.documentElement.dataset.theme==='light')
  await themeSelect.selectOption('dark')
  await page.waitForFunction(()=>window.lastSettings?.theme==='dark')
  await page.getByRole('button',{name:'Workspace',exact:true}).click()
  const darkSelection=await page.locator('.cm-selectionBackground').first().evaluate(el=>getComputedStyle(el).backgroundColor)
  assert.notEqual(darkSelection,lightSelection)
  // Outra aba usa o padrão; alterar uma página reinicia a carga na página zero.
  await page.getByRole('treeitem',{name:'banco'}).click()
  await page.getByRole('treeitem',{name:'public schema'}).click()
  await page.getByRole('treeitem',{name:'clientes'}).click()
  await page.getByText('1/500',{exact:true}).waitFor()
  await limitInput.fill('1000');await limitInput.press('Enter')
  const previewsBefore=await page.evaluate(()=>window.previewCalls)
  assert.equal(await page.evaluate(()=>window.lastPreview.rowLimit),500)
  await page.getByRole('button',{name:'Atualizar dados',exact:true}).click()
  await page.getByText('1/1000',{exact:true}).waitFor()
  assert.equal(await page.evaluate(()=>window.previewCalls),previewsBefore+1)
  assert.equal(await page.evaluate(()=>window.lastPreview.page),0)
  await page.getByRole('button',{name:'Fechar public.clientes',exact:true}).click()
  await page.getByRole('button',{name:'Fechar Nova consulta',exact:true}).click()
  await changesDialog.getByRole('button',{name:'Descartar alterações',exact:true}).click()
  await page.getByRole('button',{name:'Nova consulta',exact:true}).click()
  assert.equal(await limitInput.inputValue(),'500')
  await page.getByRole('button',{name:'Fechar Nova consulta',exact:true}).click()
  await page.reload();await editor.waitFor()
  assert.equal(await page.evaluate(()=>document.documentElement.dataset.theme),'dark')
  assert.equal(await limitInput.inputValue(),'500')
  assert.equal((await editor.locator('.cm-line').allTextContents()).join('\n'),persistedText)
  await page.getByRole('button',{name:'Abrir script',exact:true}).click()
  await libraryDialog.getByRole('button',{name:'Excluir Clientes salvos',exact:true}).click()
  const removeDialog=page.getByRole('dialog',{name:'Excluir script?',exact:true})
  await removeDialog.getByRole('button',{name:'Cancelar',exact:true}).click()
  assert.equal(savedWorkspace.scripts.length,1)
  await libraryDialog.getByRole('button',{name:'Excluir Clientes salvos',exact:true}).click()
  await removeDialog.getByRole('button',{name:'Excluir script',exact:true}).click()
  await removeDialog.waitFor({state:'hidden'})
  assert.equal(savedWorkspace.scripts.length,0)
  assert.equal(await page.locator('.tab').count(),0)
  await libraryDialog.getByRole('button',{name:'Fechar',exact:true}).click()
  await page.setViewportSize({ width: 900, height: 620 })
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false)
  const screenshot = process.env.UI_SCREENSHOT || path.join(path.dirname(fileURLToPath(import.meta.url)), '../../build/ui-validation.png')
  await page.screenshot({ path: screenshot })
  assert.deepEqual(failures, [])
  console.log('UI validada: conexões, execução, autocomplete, biblioteca, salvamento manual, limites por aba e temas persistentes.')
} finally {
  await browser.close()
}
