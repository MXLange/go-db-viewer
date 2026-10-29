import test from 'node:test'
import assert from 'node:assert/strict'
import { commandAtCursor } from '../src/commands.mjs'

test('executa somente o bloco do cursor, separado por linhas vazias', () => {
  const create = 'CREATE TABLE t (id INTEGER);'
  const insert = 'INSERT INTO t (id)\nVALUES (1), (2);'
  const select = 'SELECT id\nFROM t\nORDER BY id;'
  const text = `${create}\n\n${insert}\n\n${select}`
  assert.equal(commandAtCursor(text, 0), create)
  assert.equal(commandAtCursor(text, text.indexOf('VALUES')), insert)
  assert.equal(commandAtCursor(text, text.length), select)
  assert.equal(commandAtCursor(text, text.indexOf('\n\n') + 1), null)
})

test('somente linhas vazias separam blocos, incluindo espaços e tabs', () => {
  const text = "SELECT ';'; SELECT 2;\n \t\nSELECT 3;\n\n"
  assert.equal(commandAtCursor(text, 4), "SELECT ';'; SELECT 2;")
  assert.equal(commandAtCursor(text, text.indexOf(' \t') + 1), null)
  assert.equal(commandAtCursor(text, text.indexOf('SELECT 3')), 'SELECT 3;')
  assert.equal(commandAtCursor(text, text.length), null)
})

test('ignora documento vazio, separadores e blocos somente com comentários', () => {
  for (const text of ['', ' \t\n', '-- SELECT 1;', '/* SELECT 1; */', '/* a /* b */ c */', ';']) {
    assert.equal(commandAtCursor(text, 0), null)
  }
  assert.equal(commandAtCursor('-- explicação\nSELECT 1;', 2), '-- explicação\nSELECT 1;')
})

test('identifica o bloco pelo cursor principal sem usar a seleção', () => {
  const text = 'SELECT 1;\n\nSELECT 2;'
  assert.equal(commandAtCursor(text, text.length), 'SELECT 2;')
  for (const head of [-1, text.length + 1, 1.5, NaN]) assert.equal(commandAtCursor(text, head), null)
})

test('preserva Unicode e comandos MongoDB com várias linhas', () => {
  const first = '{\n  "find": "clientes",\n  "filter": {"nome": "João"}\n}'
  const second = '{"insert":"clientes","documents":[{"nome":"Ana"}]}'
  const text = `${first}\n\n${second}`
  assert.equal(commandAtCursor(text, text.indexOf('João'), 'json'), first)
  assert.equal(commandAtCursor(text, text.length, 'json'), second)
})

test('cursor no fim da linha pertence ao bloco anterior; próxima linha vazia não executa', () => {
  assert.equal(commandAtCursor('SELECT 1;\n', 9), 'SELECT 1;')
  assert.equal(commandAtCursor('SELECT 1;\n', 10), null)
  assert.equal(commandAtCursor('SELECT 1;\r\n\r\nSELECT 2;', 14), 'SELECT 2;')
})

test('não transforma conteúdo de literais ou comentários em outro comando', () => {
  for (const literal of ["'valor\n\nDELETE FROM clientes;'", '$tag$valor\n\nDELETE FROM clientes;$tag$', "q'[valor\n\nDELETE FROM clientes;]'"]) {
    const first = `SELECT ${literal};`
    const text = `${first}\n\nSELECT 2;`
    assert.equal(commandAtCursor(text, text.indexOf('DELETE')), first)
    assert.equal(commandAtCursor(text, text.indexOf('\n\n') + 1), first)
  }
  const comment = '/* comentário\n\nDELETE FROM clientes; */'
  assert.equal(commandAtCursor(comment, comment.indexOf('DELETE')), null)
  assert.equal(commandAtCursor('; -- sem SQL', 0), null)
  const identifiers = 'SELECT campo$tag$ FROM clientes;\n\nSELECT 2;'
  assert.equal(commandAtCursor(identifiers, identifiers.length), 'SELECT 2;')
})
