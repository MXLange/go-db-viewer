<script setup lang="ts">
import { ref, watch } from 'vue'
import type { Result, Column } from '../api'
const props = defineProps<{ result?: Result; error: string; busy: boolean; columns: Column[] }>()
const setIndex = ref(0), showStructure = ref(false)
watch(() => props.result, () => { setIndex.value = 0; showStructure.value = false })
const pretty = (text: string) => { try { return JSON.stringify(JSON.parse(text), null, 2) } catch { return text } }
</script>
<template>
  <div class="result-content" :aria-busy="busy">
    <div v-if="error" role="alert" class="result-state error-state"><span class="state-icon">!</span><h3>Falha na operação</h3><p>{{ error }}</p></div>
    <div v-else-if="busy" class="result-state" role="status"><span class="state-icon">…</span><h3>Executando</h3><p>Aguardando resposta do banco. Limite de 30 segundos.</p></div>
    <template v-else-if="result">
      <div v-if="columns.length || (result.sets?.length || 0) > 1" class="result-options">
        <button v-if="columns.length" :aria-pressed="showStructure" class="text-button" @click="showStructure = !showStructure">{{ showStructure ? 'Ver dados' : 'Ver estrutura' }}</button>
        <label v-if="(result.sets?.length || 0) > 1">Resultado <select v-model="setIndex"><option v-for="(_, i) in result.sets" :value="i">{{ i + 1 }}</option></select></label>
      </div>
      <table v-if="showStructure" class="data-table"><thead><tr><th>Campo</th><th>Tipo</th></tr></thead><tbody><tr v-for="(c, i) in columns" :key="i"><td>{{ c.name }}</td><td class="mono muted">{{ c.type }}</td></tr></tbody></table>
      <template v-else>
        <table v-if="result.sets?.[setIndex]?.columns.length" class="data-table">
          <thead><tr><th class="row-number">#</th><th v-for="(col, i) in result.sets[setIndex].columns" :key="i">{{ col.name }}<span class="column-type">{{ col.type }}</span></th></tr></thead>
          <tbody><tr v-for="(row, i) in result.sets[setIndex].rows" :key="i"><td class="row-number">{{ i + 1 }}</td><td v-for="(cell, j) in row" :key="j" :class="{ 'null-value': cell.null, numeric: /INT|DECIMAL|NUMERIC|NUMBER|FLOAT|DOUBLE|REAL/i.test(result.sets[setIndex].columns[j]?.type || '') }"><span :title="cell.truncated ? 'Valor truncado em 64 KiB' : undefined">{{ cell.null ? 'NULL' : cell.text }}{{ cell.truncated ? '…' : '' }}</span></td></tr></tbody>
        </table>
        <div v-if="result.documents?.length" class="documents"><article v-for="(doc, i) in result.documents" :key="i" class="document"><span class="document-index">DOCUMENTO {{ String(i + 1).padStart(3, '0') }}</span><pre>{{ pretty(doc) }}</pre></article></div>
        <div v-if="!result.documents?.length && !result.sets?.some(set => set.rows.length)" class="result-state"><span class="state-icon">{{ result.affected !== '' ? '✓' : '∅' }}</span><h3>{{ result.affected !== '' ? `${result.affected} linha(s) afetada(s)` : 'Nenhum registro retornado' }}</h3><p>{{ result.message || 'A operação foi concluída.' }}</p></div>
      </template>
    </template>
    <div v-else class="result-state"><span class="state-icon">↳</span><h3>Os resultados aparecem aqui</h3><p>Execute uma consulta ou abra uma tabela na barra lateral.</p></div>
  </div>
</template>
