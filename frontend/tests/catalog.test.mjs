import test from 'node:test'
import assert from 'node:assert/strict'
import { createTableCatalog } from '../src/catalog.mjs'

test('cache isola sessões, compartilha pedidos pendentes e expira em 60 segundos', async () => {
  let time = 0, calls = 0
  const catalog = createTableCatalog(async id => { calls++; return [{ name: id }] }, { now: () => time })
  await Promise.all([catalog.get('a'), catalog.get('a')])
  assert.equal(calls, 1)
  assert.equal((await catalog.get('b'))[0].name, 'b')
  assert.equal(calls, 2)
  time = 60001
  await catalog.get('a')
  assert.equal(calls, 3)
  catalog.invalidate('a'); await catalog.get('a')
  assert.equal(calls, 4)
})

test('ignora resposta invalidada enquanto carregava e permite repetir após erro', async () => {
  let resolve
  const catalog = createTableCatalog(() => new Promise(done => { resolve = done }))
  const request = catalog.get('a')
  await Promise.resolve()
  catalog.invalidate('a'); resolve([{ name: 'old' }])
  assert.equal(await request, null)
  let fail = true
  const recovering = createTableCatalog(async () => { if (fail) throw new Error('rede'); return [] })
  await assert.rejects(recovering.get('b'), /rede/)
  fail = false
  assert.deepEqual(await recovering.get('b'), [])
})

import { createMetadataCatalog } from '../src/catalog.mjs'
test('metadados compartilham resolução, isolam sessões e descartam respostas invalidadas', async () => {
  let resolves=0,describes=0,properties=0,release
  const api={
    ResolveTable:async (_id,ref)=>{resolves++;return {schema:'public',name:ref.name,kind:'table'}},
    Describe:async ()=>{describes++;return [{name:'id',type:'integer'}]},
    GetTableProperties:()=>{properties++;return new Promise(done=>release=done)},
  }
  let time=0
  const cache=createMetadataCatalog(api,{now:()=>time})
  const ref={schema:'public',name:'a',schemaQuoted:true,nameQuoted:true}
  await Promise.all([cache.columns('a',ref),cache.columns('a',ref)])
  assert.equal(resolves,1);assert.equal(describes,1)
  await cache.columns('b',ref);assert.equal(resolves,2)
  time=60001;await cache.columns('a',ref);assert.equal(resolves,3);assert.equal(describes,3)
  const pending=cache.properties('a',{schema:'public',name:'a'})
  await Promise.resolve();cache.invalidate('a');release({table:{name:'old'}})
  assert.equal(await pending,null);assert.equal(properties,1)
  await cache.columns('a',ref);assert.equal(resolves,4)
})

test('descarta falhas de metadados depois de invalidar a sessão', async()=>{
  let reject
  const cache=createMetadataCatalog({ResolveTable:()=>new Promise((_resolve,fail)=>reject=fail)})
  const pending=cache.columns('session',{name:'a'})
  await Promise.resolve();cache.invalidate('session');reject(new Error('resposta antiga'))
  assert.equal(await pending,null)
})
