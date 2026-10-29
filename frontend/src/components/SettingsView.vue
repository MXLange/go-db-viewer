<script setup lang="ts">
import { ref, watch } from 'vue'
import type { Settings } from '../api'
import { parseRowLimit } from '../settings.mjs'
const props = defineProps<{ settings: Settings; ready: boolean; status: string; error: string }>()
const emit = defineEmits<{ change: [settings: Settings]; retry: [] }>()
const rows = ref(String(props.settings.maxRows)), validation = ref('')
watch(() => props.settings.maxRows, value => { rows.value = String(value) })
function changeRows() {
  try {
    const maxRows = parseRowLimit(rows.value); validation.value = ''; rows.value = String(maxRows)
    if (maxRows !== props.settings.maxRows) emit('change', { ...props.settings, maxRows })
  } catch (error) { validation.value = (error as Error).message }
}
function changeTheme(event: Event) { emit('change', { ...props.settings, theme: (event.target as HTMLSelectElement).value as Settings['theme'] }) }
</script>
<template>
  <section class="settings-page" aria-labelledby="settings-title">
    <header><span class="eyebrow">PREFERÊNCIAS LOCAIS</span><h1 id="settings-title">Configurações</h1><p>As alterações válidas são salvas automaticamente.</p></header>
    <section class="settings-group" aria-labelledby="appearance-title"><h2 id="appearance-title">Aparência</h2><label for="theme">Tema</label><select id="theme" :value="settings.theme" :disabled="!ready" @change="changeTheme"><option value="system">Sistema</option><option value="light">Claro</option><option value="dark">Escuro</option></select><p>O modo Sistema acompanha a aparência do sistema operacional.</p></section>
    <section class="settings-group" aria-labelledby="results-settings-title"><h2 id="results-settings-title">Resultados</h2><label for="default-rows">Máximo de linhas/itens</label><input id="default-rows" v-model="rows" inputmode="numeric" :disabled="!ready" :aria-invalid="!!validation" aria-describedby="default-rows-help default-rows-error" @blur="changeRows" @keydown.enter.prevent="changeRows"><p id="default-rows-help">De 1 a 100.000. Vale para a próxima execução das abas que usam o padrão.</p><p>Na visualização de tabelas, o limite define o tamanho da página.</p><p>Resultados grandes podem atingir o limite de tamanho antes dessa quantidade.</p><p v-if="validation" id="default-rows-error" role="alert" class="danger">{{ validation }}</p></section>
    <div class="settings-status"><span role="status">{{ status }}</span><template v-if="error"><p role="alert" class="danger">{{ error }}</p><button class="secondary" @click="emit('retry')">Tentar novamente</button></template></div>
  </section>
</template>
<style scoped>
.settings-page { flex:1; min-height:0; overflow:auto; padding:28px; }
.settings-page header { margin-bottom:28px; }
h1 { font-size:28px; letter-spacing:-.02em; margin:8px 0 12px; }
h2 { font-size:18px; margin-bottom:20px; }
p { color:var(--muted); font-size:13px; line-height:1.7; }
.settings-group { padding:24px 0; border-top:1px solid var(--border); max-width:720px; }
.settings-group label { display:block; margin-bottom:8px; }
.settings-group input, .settings-group select { display:block; width:260px; max-width:100%; margin-bottom:10px; }
.settings-status { display:flex; flex-wrap:wrap; gap:12px; align-items:center; color:var(--muted); font-size:13px; }
</style>
