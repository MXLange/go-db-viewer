import test from 'node:test'
import assert from 'node:assert/strict'
import { columnCompletionContext as context, columnCompletionOptions as options, tableReferenceAt as reference } from '../src/sql-context.mjs'
const at = text => context(text,text.length)
const columns = [{name:'id',type:'integer',insertText:'"id"'},{name:'nome',type:'text',insertText:'"nome"'}]
test('resolve aliases em WHERE e ON',()=>{
  for(const text of ['SELECT * FROM "public"."clientes" C WHERE C.', 'SELECT * FROM public.clientes AS C WHERE C.', 'SELECT * FROM public.clientes C JOIN public.pedidos P ON C.']) {
    const scope=at(text)
    assert.equal(scope.relations[0].reference.name,'clientes')
    assert.equal(scope.qualified,true)
    assert.equal(scope.from,text.length)
    assert.equal(options([{relation:scope.relations[0],columns}],scope)[0].apply,'"id"')
  }
  const text='SELECT * FROM "Case"."Clientes" "Meu Alias" WHERE "Meu Alias".'
  assert.deepEqual(at(text).relations[0].reference,{schema:'Case',name:'Clientes',schemaQuoted:true,nameQuoted:true})
})
test('sem alias inclui JOIN e qualifica nomes duplicados',()=>{
  const scope=at('SELECT * FROM clientes C JOIN pedidos P ON C.id=P.id WHERE ')
  assert.equal(scope.relations.length,2)
  const result=options([{relation:scope.relations[0],columns},{relation:scope.relations[1],columns:[columns[0],{name:'valor',type:'numeric'}]}],scope)
  assert.deepEqual(result.map(c=>c.apply),['C."id"','"nome"','P."id"','"valor"'])
  const text='SELECT * FROM clientes WHERE '
  assert.equal(context(text,text.length,false),null)
})
test('descobre FROM posterior ao cursor e filtra prefixos',()=>{
  const text='SELECT C.no FROM clientes C',scope=context(text,text.indexOf('no')+2)
  assert.equal(scope.relations[0].reference.name,'clientes')
  assert.equal(options([{relation:scope.relations[0],columns}],scope)[0].apply,'"nome"')
})
test('ignora literais, comentários, números, outro bloco e alias desconhecido',()=>{
  for(const text of ["SELECT * FROM clientes C WHERE 'C.",'SELECT * FROM clientes C WHERE $$C.','SELECT * FROM clientes C WHERE -- C.','SELECT * FROM clientes C WHERE /* C.','SELECT * FROM clientes C WHERE 1.','SELECT * FROM clientes C\n\nWHERE C.','SELECT * FROM clientes C; SELECT C.','SELECT * FROM clientes C WHERE X.']) assert.equal(at(text),null,text)
})
test('CTE e tabela derivada não resolvem como tabela física',()=>{
  for(const text of ['WITH clientes AS (SELECT * FROM real) SELECT * FROM clientes C WHERE C.','SELECT * FROM (SELECT * FROM real) C WHERE C.','SELECT * FROM real C WHERE EXISTS (SELECT * FROM (SELECT 1) C WHERE C.']) assert.equal(at(text),null,text)
})
test('subquery sombreia alias externo e mantém correlação',()=>{
  const text='SELECT * FROM clientes C WHERE EXISTS (SELECT * FROM pedidos C WHERE C.)'
  assert.equal(context(text,text.indexOf('C.)')+2).relations[0].reference.name,'pedidos')
  const correlated='SELECT * FROM clientes C WHERE EXISTS (SELECT * FROM pedidos P WHERE C.)'
  assert.equal(context(correlated,correlated.indexOf('C.)')+2).relations[0].reference.name,'clientes')
})
test('Ctrl+clique resolve nome, alias e qualificador',()=>{
  const text='SELECT * FROM "public"."clientes" C WHERE C.nome = 1'
  for(const position of [text.indexOf('public')+2,text.indexOf('clientes')+2,text.indexOf(' C ')+1,text.indexOf('C.nome')]) assert.equal(reference(text,position).name,'clientes')
  assert.equal(reference(text,text.indexOf('nome')+1),null)
  const literal="SELECT * FROM clientes C WHERE nome='C.nome'"
  assert.equal(reference(literal,literal.lastIndexOf('C.nome')+1),null)
})

test('respeita case e delimitadores conforme o banco',()=>{
  assert.equal(at('SELECT * FROM clientes C WHERE "C".'),null)
  assert.ok(at('SELECT * FROM clientes C WHERE "c".'))
  const oracle='SELECT * FROM CLIENTES C WHERE "C".'
  assert.ok(context(oracle,oracle.length,true,'oracle'))
  const server='SELECT * FROM [Clientes] [Meu Alias] WHERE [meu alias].'
  const scope=context(server,server.length,true,'sqlserver')
  assert.ok(scope)
  const unqualified='SELECT * FROM [Clientes] [Meu Alias] JOIN [Pedidos] P ON '
  const ctx=context(unqualified,unqualified.length,true,'sqlserver')
  const result=options(ctx.relations.map(relation=>({relation,columns:[columns[0]]})),ctx)
  assert.equal(result[0].apply,'[Meu Alias]."id"')
})
test('limita relações por braço de UNION e rejeita alias ambíguo',()=>{
  const text='SELECT * FROM clientes C WHERE C.id=1 UNION SELECT * FROM pedidos C WHERE C.'
  assert.equal(at(text).relations[0].reference.name,'pedidos')
  assert.equal(at('SELECT * FROM clientes C JOIN pedidos C ON C.'),null)
})

test('não permite correlação com o SELECT externo em CTE ou tabela derivada',()=>{
  for(const text of [
    'WITH foo AS (SELECT * FROM pedidos P WHERE C.) SELECT * FROM clientes C',
    'SELECT * FROM (SELECT * FROM pedidos P WHERE C.) X JOIN clientes C ON 1=1',
  ]) assert.equal(context(text,text.indexOf('C.)')+2),null,text)
})
test('schema explícito seleciona apenas a tabela correspondente',()=>{
  const text='SELECT * FROM public.clientes JOIN sales.clientes ON public.clientes.'
  assert.equal(at(text).relations[0].reference.schema,'public')
  assert.equal(at('SELECT * FROM public.clientes WHERE sales.clientes.'),null)
  const plain='SELECT * FROM public.clientes JOIN sales.clientes ON '
  const scope=at(plain)
  assert.deepEqual(options(scope.relations.map(relation=>({relation,columns:[columns[0]]})),scope).map(o=>o.apply),['public.clientes."id"','sales.clientes."id"'])
})
