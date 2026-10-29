import test from 'node:test'
import assert from 'node:assert/strict'
import { tableCompletionContext, tableCompletionOptions } from '../src/completions.mjs'

const scope = text => tableCompletionContext(text, text.length)
const tables = [
  { schema: 'public', name: 'clientes', insertText: '"public"."clientes"' },
  { schema: 'sales', name: 'clientes', insertText: '"sales"."clientes"' },
  { schema: 'Sales Team', name: 'Order"Detail', insertText: '"Sales Team"."Order""Detail"' },
]

test('Ctrl+Espaço após FROM e JOIN oferece nomes qualificados de todos os schemas', () => {
  for (const text of ['SELECT * FROM ', 'select * from\n', 'SELECT * FROM public.a JOIN ']) {
    assert.equal(tableCompletionOptions(tables, scope(text)).length, 3)
  }
  const text = 'SELECT * FROM cli', context = scope(text)
  assert.equal(context.from, text.indexOf('cli'))
  assert.deepEqual(tableCompletionOptions(tables, context).map(t => t.label), ['public.clientes', 'sales.clientes'])
  assert.equal(tableCompletionOptions(tables, scope('SELECT * FROM'))[0].apply, ' "public"."clientes"')
})

test('filtra por schema, preserva nomes especiais e substitui todo o prefixo', () => {
  const context = scope('SELECT * FROM public.cl')
  assert.equal(context.schema, 'public')
  assert.deepEqual(tableCompletionOptions(tables, context).map(t => t.apply), ['"public"."clientes"'])
  const quoted = scope('SELECT * FROM "Sales Team"."Order')
  assert.equal(tableCompletionOptions(tables, quoted)[0].apply, '"Sales Team"."Order""Detail"')
  const mssql = scope('SELECT * FROM [Sales Team].[Order')
  assert.equal(tableCompletionOptions(tables, mssql).length, 1)
  assert.equal(tableCompletionOptions(tables, scope('SELECT * FROM missing.')).length, 0)
})

test('não sugere tabelas em strings, comentários, aliases ou outro bloco', () => {
  for (const text of [
    "SELECT 'FROM cli", 'SELECT $$FROM cli', "SELECT q'[FROM cli", '-- SELECT * FROM ', '/* SELECT * FROM ',
    'SELECT * FROM t WHERE nome=', 'SELECT * FROM t alias ', 'SELECT * FROM\n\n', "SELECT 'FROM cli'", 'SELECT 1;',
  ]) assert.equal(scope(text), null, text)
  assert.ok(scope('/* FROM fake */ SELECT * FROM '))
  assert.ok(scope('WITH t AS (SELECT 1) SELECT * FROM '))
})
