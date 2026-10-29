<script setup lang="ts">
import { ref } from 'vue'
import { api, errorMessage, type DBNode } from '../api'
const props = defineProps<{ node: DBNode; session: string; level?: number }>()
const emit = defineEmits<{ open: [node: DBNode]; database: [database: string] }>()
const expanded = ref(false), loading = ref(false), error = ref('')
const children = ref<DBNode[] | null>(null)
const expandable = () => props.node.kind === 'database' || props.node.kind === 'schema'
async function activate() {
  if (!expandable()) { emit('open', props.node); return }
  if (props.node.kind === 'database') emit('database', props.node.name)
  expanded.value = !expanded.value
  if (!expanded.value || children.value) return
  loading.value = true; error.value = ''
  try { children.value = await api.List(props.session, props.node) || [] }
  catch (e) { error.value = errorMessage(e); expanded.value = false }
  finally { loading.value = false }
}
</script>
<template>
  <li role="none">
    <button class="tree-node" role="treeitem" :aria-expanded="expandable() ? expanded : undefined" :aria-level="(level || 0) + 1" :style="{ paddingLeft: `${12 + (level || 0) * 14}px` }" :disabled="loading" @click="activate">
      <span class="tree-chevron" aria-hidden="true">{{ loading ? '·' : expandable() ? (expanded ? '⌄' : '›') : '▤' }}</span>
      <span class="tree-label">{{ node.name }}</span><span class="node-kind">{{ node.kind === 'collection' ? 'coleção' : node.kind === 'schema' ? 'schema' : '' }}</span>
    </button>
    <p v-if="error" class="tree-error" role="alert">{{ error }}</p>
    <ul v-if="expanded && children" role="group" class="tree-list">
      <TreeNode v-for="child in children" :key="`${child.kind}:${child.name}`" :node="child" :session="session" :level="(level || 0) + 1" @open="emit('open', $event)" @database="emit('database', $event)" />
      <li v-if="!children.length" class="tree-empty">Nenhum objeto disponível.</li>
    </ul>
  </li>
</template>
