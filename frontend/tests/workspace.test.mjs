import test from 'node:test'
import assert from 'node:assert/strict'
import { setImmediate as checkpoint } from 'node:timers/promises'
import { serializeWorkspace, restoreWorkspace, createWorkspaceSaver, scriptDirty, scriptSnapshot, scriptList } from '../src/workspace.mjs'
const saved = (id='a',title='Clientes',profileID='saved') => ({id,title,text:'SELECT 1;',kind:'postgresql',profileID,connectionName:'Local',database:'dbviewer',updatedAt:'2026-10-29T12:00:00Z'})
const tab = (id=1,script=saved()) => ({...script,id,scriptID:script.id,anchor:1,head:4,sessionID:'transient',busy:false,disconnected:false,result:{password:'do-not-save'},dsn:'do-not-save'})
const snapshot = head => serializeWorkspace([{...tab(),head}],1)
const timers = () => ({setTimer:()=>1,clearTimer:()=>{}})
test('estado das abas nunca contém conteúdo, nome ou credenciais',()=>{
 const a=tab(3),b=tab(7,saved('b')),draft={...tab(8),scriptID:undefined},preview={...tab(9),node:{}}
 const state=serializeWorkspace([a,b,draft,preview],7)
 assert.deepEqual(state,{version:2,activeTabID:7,openTabs:[{id:3,scriptID:'a',anchor:1,head:4},{id:7,scriptID:'b',anchor:1,head:4}]})
 assert.equal(JSON.stringify(state).includes('SELECT'),false)
 const restored=restoreWorkspace({...state,scripts:[saved(),saved('b')]})
 assert.equal(restored[0].text,'SELECT 1;');assert.equal(restored[0].sessionID,'');assert.equal(restored[0].disconnected,true)
 assert.equal(restored[0].result,undefined)
})
test('restaura somente a versão salva e limita o cursor',()=>{
 const edited={...tab(),text:'alteração pendente',head:999}
 const state=serializeWorkspace([edited],1)
 const restored=restoreWorkspace({...state,scripts:[saved()]})
 assert.equal(restored[0].text,'SELECT 1;');assert.equal(restored[0].head,9)
 assert.deepEqual(serializeWorkspace([{...tab(),node:{},propertiesView:true}],1),{version:2,activeTabID:0,openTabs:[]})
})
test('dirty acompanha conteúdo, banco, vínculo e o snapshot confirmado',()=>{
 const library=[saved()],a=tab()
 assert.equal(scriptDirty(a,library),false)
 a.text='SELECT 2;';assert.equal(scriptDirty(a,library),true)
 const snapshot=scriptSnapshot(a,'Novo nome')
 a.text='SELECT 3;';library[0]={...snapshot,id:'a'}
 assert.equal(scriptDirty(a,library),true)
 a.text=snapshot.text;assert.equal(scriptDirty(a,library),false)
 a.database='other';assert.equal(scriptDirty(a,library),true)
 const draft={...tab(),scriptID:undefined,initialText:'SELECT 1;'}
 assert.equal(scriptDirty(draft,library),false);draft.text='alterado';assert.equal(scriptDirty(draft,library),true)
 assert.equal(scriptDirty({...draft,node:{}},library),false)
 assert.equal(scriptSnapshot(draft,'A').id,'')
})
test('biblioteca filtra conexão, nome e scripts órfãos em ordem de atualização',()=>{
 const profiles=[{id:'one'}],a=saved('a','Clientes','one'),b={...saved('b','Pedidos','gone'),updatedAt:'2026-10-29T15:00:00Z'},c=saved('c','Clientes','')
 assert.deepEqual(scriptList([a,b,c],profiles,'one').map(s=>s.id),['a'])
 assert.deepEqual(scriptList([a,b,c],profiles,'$unlinked').map(s=>s.id),['b','c'])
 assert.deepEqual(scriptList([a,b,c],profiles,'$all','CLIENT').map(s=>s.id),['a','c'])
})
test('debounce e flush usam o último estado e copiam o snapshot',async()=>{
 const writes=[],states=[],saver=createWorkspaceSaver(async state=>writes.push(state),status=>states.push(status),timers())
 saver.schedule(snapshot(1));const latest=snapshot(2);saver.schedule(latest);latest.openTabs[0].head=999
 await saver.flush();assert.equal(writes.length,1);assert.equal(writes[0].openTabs[0].head,2);assert.equal(states.at(-1),'Salvo')
 await saver.flush();assert.equal(writes.length,1)
})
test('gravações lentas mantêm a ordem e permitem repetir falhas',async()=>{
 const pending=[],states=[],saver=createWorkspaceSaver(data=>new Promise(resolve=>pending.push({data,resolve})),state=>states.push(state),timers())
 saver.schedule(snapshot(1));const first=saver.flush();await checkpoint()
 saver.schedule(snapshot(2));const second=saver.flush();await checkpoint()
 assert.equal(pending.length,1);pending[0].resolve();await first;await checkpoint()
 assert.equal(states.at(-1),'Salvando');assert.equal(pending[1].data.openTabs[0].head,2)
 pending[1].resolve();await second;assert.equal(states.at(-1),'Salvo')
 let fail=true,calls=0
 const recovering=createWorkspaceSaver(async()=>{calls++;if(fail)throw new Error('disco')},()=>{},timers())
 recovering.schedule(snapshot(1));await assert.rejects(recovering.flush(),/disco/);fail=false;await recovering.flush();assert.equal(calls,2)
})
