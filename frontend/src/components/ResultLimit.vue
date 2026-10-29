<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Result } from '../api'
import { parseRowLimit, resultCounter } from '../settings.mjs'
const props = defineProps<{ result?: Result; limit: number; overridden: boolean; defaultLimit: number; disabled: boolean; preview?: boolean }>()
const emit = defineEmits<{ change: [limit: number]; reset: [] }>()
const value = ref(String(props.limit)), error = ref(''), edited = ref(false)
watch(() => props.limit, limit => { value.value = String(limit); error.value = ''; edited.value = false })
const pending = computed(() => props.result?.rowLimit ? props.result.rowLimit !== props.limit : props.overridden)
function commit() {
  if (!edited.value) return
  try {
    const limit = parseRowLimit(value.value.replace(/^\//, '')); error.value = ''; value.value = String(limit); edited.value = false
    if (limit !== props.limit || !props.overridden) emit('change', limit)
  } catch (failure) { error.value = (failure as Error).message }
}
</script>
<template>
  <div class="result-limit">
    <span v-if="result?.rowLimit" class="result-count" role="status">{{ resultCounter(result) }}<span v-if="(result.sets?.length || 0) > 1"> · total da execução</span></span>
    <label>Máximo nesta aba <span class="limit-value">/<input v-model="value" @input="edited = true" aria-label="Máximo nesta aba" inputmode="numeric" :disabled="disabled" :aria-invalid="!!error" @blur="commit" @keydown.enter.prevent="commit"></span></label>
    <button v-if="overridden" class="text-button" :disabled="disabled" @click="emit('reset')">Usar padrão ({{ defaultLimit }})</button>
    <span v-else class="muted">Padrão global</span>
    <span v-if="pending" class="muted" role="status">{{ preview ? 'Novo limite na próxima carga, a partir da primeira página.' : 'Novo limite na próxima execução.' }}</span>
    <span v-if="result?.limitReason === 'bytes'" class="limit-warning" role="status">Retorno interrompido pelo limite de tamanho.</span>
    <span v-if="error" role="alert" class="danger">{{ error }}</span>
  </div>
</template>
<style scoped>
.result-limit { flex-shrink:0; display:flex; flex-wrap:wrap; align-items:center; gap:8px 16px; padding:8px 16px; background:var(--surface); border-top:1px solid var(--border); font-size:12px; }
label, .limit-value { display:inline-flex; gap:6px; align-items:center; }
input { width:88px; min-height:30px; font:12px var(--font-mono); }
.limit-warning { color:var(--warning); }
</style>
