<script setup lang="ts">
import { computed } from 'vue'
import type { TableProperties } from '../api'
const props = defineProps<{ properties?: TableProperties; busy: boolean; error: string; disconnected: boolean }>()
defineEmits<{ refresh: [] }>()
const sections = computed(() => props.properties ? [
  { title: 'Colunas', value: props.properties.columns },
  { title: 'Chave primária', value: props.properties.primaryKey },
  { title: 'Chaves estrangeiras', value: props.properties.foreignKeys },
  { title: 'Índices', value: props.properties.indexes },
] : [])
</script>
<template>
  <section class="table-properties" aria-label="Propriedades da tabela" :aria-busy="busy">
    <header class="result-toolbar"><div><span class="section-label">PROPRIEDADES</span><p v-if="properties">{{ properties.table.schema }}.{{ properties.table.name }}</p></div><button class="secondary" :disabled="busy || disconnected" @click="$emit('refresh')">Atualizar propriedades</button></header>
    <p v-if="error" role="alert" class="danger">{{ error }}</p>
    <p v-if="busy" role="status">Carregando propriedades…</p>
    <div v-for="section in sections" :key="section.title" class="property-section">
      <h2>{{ section.title }}</h2>
      <p v-if="section.value.status !== 'available'" role="status" class="muted">{{ section.value.message }}</p>
      <p v-else-if="!section.value.rows?.length" class="muted">Nenhum item encontrado.</p>
      <div v-else class="property-scroll"><table><thead><tr><th v-for="(column, i) in section.value.columns" :key="i" scope="col">{{ column }}</th></tr></thead><tbody><tr v-for="(row, i) in section.value.rows" :key="i"><td v-for="(cell, j) in row" :key="j" :class="{ muted: cell.null }">{{ cell.null ? 'NULL' : cell.text }}{{ cell.truncated ? '…' : '' }}</td></tr></tbody></table></div>
    </div>
  </section>
</template>
<style scoped>
.table-properties { flex: 1; min-height: 0; overflow: auto; padding: 0 24px 24px; }
.table-properties > .result-toolbar { padding-left: 0; padding-right: 0; }
.property-section { margin-top: 24px; }
h2 { font-size: 15px; font-weight: 700; margin: 0 0 12px; }
.property-scroll { overflow: auto; border: 1px solid var(--border); border-radius: 6px; }
table { width: 100%; border-collapse: collapse; text-align: left; font-family: var(--font-mono); font-size: 12px; }
th { color: var(--muted); background: var(--surface); font-weight: 600; }
th, td { padding: 10px 12px; border-bottom: 1px solid var(--border); white-space: pre-wrap; min-width: 110px; overflow-wrap: anywhere; }
tr:last-child td { border-bottom: 0; }
</style>
