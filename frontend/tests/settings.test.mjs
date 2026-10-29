import test from 'node:test'
import assert from 'node:assert/strict'
import { defaultSettings, parseRowLimit, effectiveRowLimit, previewPage, resultCounter, resultItemCount, effectiveTheme, applyTheme } from '../src/settings.mjs'
import { serializeWorkspace, scriptSnapshot, scriptDirty } from '../src/workspace.mjs'

test('padrão de 200 e validação de limites inteiros',()=>{
 assert.deepEqual(defaultSettings(),{theme:'system',maxRows:200})
 for(const value of ['1','200','1000','100000',' 1000 '])assert.equal(parseRowLimit(value),Number(value))
 for(const value of ['',0,-1,1.5,'1e3','1,000','200x',100001,'Infinity'])assert.throws(()=>parseRowLimit(value),/limite inteiro/)
})
test('limites são temporários e isolados por aba',()=>{
 const settings=defaultSettings(),a={rowLimit:1000},b={}
 assert.equal(effectiveRowLimit(a,settings),1000);assert.equal(effectiveRowLimit(b,settings),200)
 settings.maxRows=500;assert.equal(effectiveRowLimit(a,settings),1000);assert.equal(effectiveRowLimit(b,settings),500)
 a.rowLimit=undefined;assert.equal(effectiveRowLimit(a,settings),500)
 const tab={id:1,scriptID:'saved',rowLimit:1000,text:'SELECT 1',initialText:'SELECT 1',title:'A',database:'db',profileID:'p'}
 assert.equal('rowLimit' in serializeWorkspace([tab],1).openTabs[0],false)
 assert.equal('rowLimit' in scriptSnapshot(tab,'A'),false)
 assert.equal(scriptDirty(tab,[{id:'saved',text:tab.text,database:tab.database,profileID:tab.profileID}]),false)
})
test('rodapé conserva o limite usado e identifica exatamente o máximo',()=>{
 const make=(count,limit=200)=>({rowLimit:limit,sets:[{rows:Array(count).fill([])}]})
 assert.equal(resultCounter(make(0)),'0/200')
 assert.equal(resultCounter(make(37)),'37/200')
 assert.equal(resultCounter(make(200)),'200/200 máx')
 assert.equal(resultCounter({rowLimit:1000,documents:Array(1000).fill('{}')}),'1000/1000 máx')
 assert.equal(resultCounter({rowLimit:0,documents:['{"ok":1}']}),'')
 const multiple={rowLimit:200,sets:[{rows:Array(100).fill([])},{rows:Array(100).fill([])}]}
 assert.equal(resultItemCount(multiple),200);assert.equal(resultCounter(multiple),'200/200 máx')
 const tab={rowLimit:1000,result:make(37)}
 assert.equal(resultCounter(tab.result),'37/200')
 assert.equal(previewPage(tab,3,1000),0)
 assert.equal(previewPage(tab,3,200),3)
})
test('temas explícitos e sistema acompanham a preferência atual',()=>{
 assert.equal(effectiveTheme('system',true),'dark');assert.equal(effectiveTheme('system',false),'light')
 assert.equal(effectiveTheme('light',true),'light');assert.equal(effectiveTheme('dark',false),'dark')
 const root={dataset:{},style:{}}
 applyTheme('light',root);assert.equal(root.dataset.theme,'light');assert.equal(root.style.colorScheme,'light')
 applyTheme('dark',root);assert.equal(root.dataset.theme,'dark')
})
