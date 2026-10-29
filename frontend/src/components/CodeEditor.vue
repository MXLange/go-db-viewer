<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { EditorView, keymap } from '@codemirror/view'
import { Compartment, EditorState, Prec } from '@codemirror/state'
import { basicSetup } from 'codemirror'
import { autocompletion, acceptCompletion, closeCompletion, startCompletion } from '@codemirror/autocomplete'
import { commandAtCursor } from '../commands.mjs'
import { tableCompletionContext, tableCompletionOptions } from '../completions.mjs'
import { columnCompletionContext, columnCompletionOptions, tableReferenceAt } from '../sql-context.mjs'
import type { Column, TableReference, TableSuggestion } from '../api'
const props = defineProps<{ modelValue: string; disabled: boolean; language: string; dialect?: string; connected?: boolean; sessionID?: string; anchor?: number; head?: number; loadTables?: (sessionID: string) => Promise<TableSuggestion[] | null>; loadColumns?: (sessionID: string, reference: TableReference) => Promise<Column[] | null> }>()
const emit = defineEmits<{ 'update:modelValue': [value: string]; execute: [text: string]; cursor: [selection: { anchor: number; head: number }]; completionError: [error: unknown]; properties: [event: { sessionID: string; reference: TableReference }] }>()
const host = ref<HTMLDivElement>()
const readOnly = new Compartment()
let view: EditorView | undefined
function executeCurrentCommand() {
  if (!view?.hasFocus || props.disabled || props.connected === false) return
  const text = commandAtCursor(view.state.doc.toString(), view.state.selection.main.head, props.language)
  if (text) emit('execute', text)
}
defineExpose({ executeCurrentCommand })
async function completeTables(context: import('@codemirror/autocomplete').CompletionContext) {
  const sessionID = props.sessionID
  if (props.disabled || !props.connected || props.language !== 'sql' || !sessionID || !props.loadTables) return null
  const document = context.state.doc.toString()
  const tableScope = context.explicit ? tableCompletionContext(document, context.pos) : null
  const columnScope = tableScope ? null : columnCompletionContext(document, context.pos, context.explicit, props.dialect)
  if (!tableScope && !columnScope) return null
  const current = () => view?.hasFocus && !context.aborted && props.sessionID === sessionID && props.connected && !props.disabled && view.state.doc === context.state.doc && view.state.selection.main.head === context.pos
  try {
    if (tableScope) {
      const tables = await props.loadTables(sessionID)
      if (!tables || !current()) return null
      return { from: tableScope.from, options: tableCompletionOptions(tables, tableScope), filter: false }
    }
    if (!props.loadColumns || !columnScope) return null
    const entries = await Promise.all(columnScope.relations.map(async (relation: any) => ({ relation, columns: await props.loadColumns!(sessionID, relation.reference) })))
    if (!current() || entries.some(entry => entry.columns == null)) return null
    return { from: columnScope.from, options: columnCompletionOptions(entries, { ...columnScope, prefix: '' }), validFor: /^[\p{L}\p{N}_$#]*$/u }
  } catch (error) { if (current()) emit('completionError', error); return null }
}

onMounted(() => {
  view = new EditorView({ parent: host.value, state: EditorState.create({
    doc: props.modelValue,
    selection: { anchor: Math.min(props.modelValue.length, props.anchor || 0), head: Math.min(props.modelValue.length, props.head || 0) },
    extensions: [basicSetup, readOnly.of(EditorState.readOnly.of(props.disabled)),
      autocompletion({ override: [completeTables], activateOnTyping: false }),
      EditorView.domEventHandlers({ mousedown(event, editor) {
        if (event.button !== 0 || !(event.ctrlKey || event.metaKey) || props.disabled || !props.connected || props.language !== 'sql' || !props.sessionID) return false
        const position = editor.posAtCoords({ x: event.clientX, y: event.clientY })
        if (position == null) return false
        const reference = tableReferenceAt(editor.state.doc.toString(), position, props.dialect)
        if (!reference) return false
        event.preventDefault()
        emit('properties', { sessionID: props.sessionID, reference })
        return true
      } }),
      EditorView.contentAttributes.of(view => ({ 'aria-label': 'Editor de consulta', spellcheck: 'false', 'aria-readonly': String(view.state.readOnly) })),
      Prec.highest(keymap.of([
        { key: 'Ctrl-Enter', run: () => { executeCurrentCommand(); return true } },
        { key: 'Mod-Enter', run: () => { executeCurrentCommand(); return true } },
        { key: 'Tab', run: acceptCompletion },
      ])),
      EditorView.updateListener.of(update => {
        if (update.docChanged) {
          emit('update:modelValue', update.state.doc.toString())
          if (update.state.selection.main.head > 0 && update.transactions.some(t => t.isUserEvent('input.type')) && update.state.selection.main.empty && update.state.doc.sliceString(update.state.selection.main.head - 1, update.state.selection.main.head) === '.') {
            queueMicrotask(() => { if (view && !props.disabled && props.connected) startCompletion(view) })
          }
        }
        if (update.docChanged || update.selectionSet) { const { anchor, head } = update.state.selection.main; emit('cursor', { anchor, head }) }
      }),
      EditorView.theme({ '&': { height: '100%', backgroundColor: 'var(--bg)', color: 'var(--fg)' },
        '.cm-scroller': { fontFamily: 'var(--font-mono)', fontSize: '13px', lineHeight: '1.8', overflow: 'auto' },
        '.cm-content': { padding: '16px 0' }, '.cm-gutters': { backgroundColor: 'var(--bg)', color: 'var(--muted)', border: 'none', paddingRight: '12px' },
        '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'var(--surface)' },
        '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection': { backgroundColor: 'var(--selection)' },
        '.cm-cursor': { borderLeftColor: 'var(--accent)' }, '&.cm-focused': { outline: 'none' },
        '.cm-tooltip-autocomplete': { backgroundColor: 'var(--surface)', color: 'var(--fg)', border: '1px solid var(--border)' },
        '.cm-tooltip-autocomplete ul li[aria-selected]': { backgroundColor: 'var(--selection)', color: 'var(--success)' },
      }, { dark: true }),
    ],
  }) })
})
watch(() => props.modelValue, value => {
  if (view && view.state.doc.toString() !== value) view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } })
})
watch(() => props.disabled, value => view?.dispatch({ effects: readOnly.reconfigure(EditorState.readOnly.of(value)) }))
watch(() => [props.sessionID, props.connected, props.disabled], () => { if (view) closeCompletion(view) })
onBeforeUnmount(() => view?.destroy())
</script>
<template><div ref="host" class="code-editor" :data-language="language" /></template>
