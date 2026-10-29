import test from 'node:test'
import assert from 'node:assert/strict'
import { applyResult, disconnectTabs } from '../src/tabs.mjs'

test('resposta permanece na aba e sessão de origem', () => {
  const a = { sessionID: 'a', error: 'old' }, b = { sessionID: 'b' }
  assert.equal(applyResult(a, 'b', { affected: '5' }), false)
  assert.equal(a.result, undefined)
  assert.equal(applyResult(a, 'a', { affected: '9007199254740993' }), true)
  assert.equal(a.result.affected, '9007199254740993')
  assert.equal(b.result, undefined)
})

test('desconexão invalida somente suas abas e ignora respostas atrasadas', () => {
  const a = { sessionID: 'a', busy: true }, b = { sessionID: 'b', busy: true }
  disconnectTabs([a, b], 'a')
  assert.equal(a.disconnected, true)
  assert.equal(a.busy, false)
  assert.equal(b.busy, true)
  assert.equal(applyResult(a, 'a', { affected: '1' }), false)
})
