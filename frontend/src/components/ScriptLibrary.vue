<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { SavedScript, Profile, DriverInfo, Kind } from '../api'
import { scriptList } from '../workspace.mjs'
const props = defineProps<{ scripts: SavedScript[]; profiles: Profile[]; drivers: DriverInfo[]; profileID: string; locked: boolean }>()
const emit = defineEmits<{ open: [script: SavedScript]; create: [target: { profileID: string; kind: Kind }]; rename: [script: SavedScript]; remove: [script: SavedScript]; close: [] }>()
const search = ref(''), filter = ref(props.profileID || '$all'), kind = ref<Kind>('postgresql')
watch(() => props.profileID, value => { filter.value = value || '$all'; search.value = '' })
const listed = computed(() => scriptList(props.scripts, props.profiles, filter.value, search.value))
function create() {
  const profile = props.profiles.find((p: Profile) => p.id === filter.value)
  emit('create', { profileID: profile?.id || '', kind: profile?.kind || kind.value })
}
</script>
<template>
  <header class="dialog-header"><div><span class="eyebrow">BIBLIOTECA</span><h2 id="script-library-title">Scripts salvos</h2></div><button class="icon-button" aria-label="Fechar biblioteca" @click="emit('close')">×</button></header>
  <div class="dialog-body script-library-body">
    <div class="form-row"><label>Conexão dos scripts<select v-model="filter"><option value="$all">Todas as conexões</option><option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{ profile.name }}</option><option value="$unlinked">Sem conexão salva</option></select></label><label>Buscar script<input v-model="search" type="search" placeholder="Nome do script"></label></div>
    <div class="library-create"><label v-if="filter === '$unlinked'" class="compact-label">Tipo do banco<select v-model="kind"><option v-for="driver in drivers" :key="driver.kind" :value="driver.kind">{{ driver.name }}</option></select></label><span v-if="filter === '$all'" class="muted">Escolha uma conexão para criar um script.</span><button class="primary" :disabled="locked || filter === '$all'" @click="create">＋ Novo script</button></div>
    <p v-if="!listed.length" role="status" class="muted">{{ search ? 'Nenhum script encontrado.' : 'Nenhum script salvo nesta lista.' }}</p>
    <ul v-else class="script-library-list" aria-label="Lista de scripts">
      <li v-for="script in listed" :key="script.id"><button class="script-open" :disabled="locked" @click="emit('open', script)"><strong>{{ script.title }}</strong><small>{{ script.connectionName || 'Sem conexão salva' }} · {{ new Date(script.updatedAt).toLocaleString('pt-BR') }}</small></button><button class="text-button" :disabled="locked" :aria-label="`Renomear ${script.title}`" @click="emit('rename', script)">Renomear</button><button class="text-button" :disabled="locked" :aria-label="`Excluir ${script.title}`" @click="emit('remove', script)">Excluir</button></li>
    </ul>
  </div>
  <footer class="dialog-footer"><span class="muted">Scripts locais. Salve com Ctrl+S.</span><button class="secondary" @click="emit('close')">Fechar</button></footer>
</template>
<style scoped>
.script-library-body { min-height: 260px; }
.library-create { display:flex; align-items:center; justify-content:space-between; gap:12px; margin:20px 0; }
.script-library-list { list-style:none; padding:0; margin:0; max-height:40vh; overflow:auto; }
.script-library-list li { display:flex; align-items:center; gap:8px; border-top:1px solid var(--border); }
.script-open { flex:1; min-width:0; text-align:left; padding:14px 8px; background:transparent; border:0; color:var(--fg); }
.script-open:hover { background:var(--surface); }
.script-open strong, .script-open small { display:block; overflow-wrap:anywhere; }
.script-open small { color:var(--muted); margin-top:4px; }
</style>
