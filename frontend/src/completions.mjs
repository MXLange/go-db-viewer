// O contexto usa somente tokens fora de strings e comentários.
export function tableCompletionContext(document, position) {
  const source = document.slice(0, position), tokens = []
  const name = /[\p{L}\p{N}_$#]/u
  let i = 0
  while (i < source.length) {
    const start = i, c = source[i]
    if (/\s/.test(c)) {
      while (i < source.length && /\s/.test(source[i])) i++
      if (/\n[\t \r]*\n/.test(source.slice(start, i))) tokens.length = 0
      continue
    }
    if (source.startsWith('--', i)) {
      const end = source.indexOf('\n', i + 2)
      if (end < 0) return null
      i = end; continue
    }
    if (source.startsWith('/*', i)) {
      i += 2; let depth = 1
      while (i < source.length && depth) {
        if (source.startsWith('/*', i)) { depth++; i += 2 }
        else if (source.startsWith('*/', i)) { depth--; i += 2 }
        else i++
      }
      if (depth) return null
      continue
    }
    if ((c === 'q' || c === 'Q') && source[i + 1] === "'" && source[i + 2]) {
      const open = source[i + 2], close = ({ '[': ']', '{': '}', '(': ')', '<': '>' }[open] || open) + "'"
      const end = source.indexOf(close, i + 3)
      if (end < 0) return null
      i = end + close.length; tokens.push({ type: 'literal' }); continue
    }
    if (c === '$') {
      const tag = /^\$[a-zA-Z0-9_]*\$/.exec(source.slice(i))?.[0]
      if (tag) {
        const end = source.indexOf(tag, i + tag.length)
        if (end < 0) return null
        i = end + tag.length; tokens.push({ type: 'literal' }); continue
      }
    }
    if (c === "'" || c === '"' || c === '[' || c === '`') {
      const close = c === '[' ? ']' : c
      const escaped = c === "'" && /e/i.test(source[i - 1] || '') && !name.test(source[i - 2] || '')
      let value = '', closed = false; i++
      while (i < source.length) {
        if (escaped && source[i] === '\\') { value += source.slice(i, i + 2); i += 2 }
        else if (source[i] === close) {
          if (source[i + 1] === close) { value += close; i += 2 }
          else { i++; closed = true; break }
        } else value += source[i++]
      }
      if (c === "'" && !closed) return null
      tokens.push({ type: c === "'" ? 'literal' : 'identifier', value, from: start, to: i }); continue
    }
    if (/[\p{L}_]/u.test(c)) {
      i++
      while (i < source.length && name.test(source[i])) i++
      tokens.push({ type: 'word', value: source.slice(start, i), from: start, to: i }); continue
    }
    if (c === ';') tokens.length = 0
    else tokens.push({ type: 'symbol', value: c, from: start, to: start + 1 })
    i++
  }
  for (let j = tokens.length - 1; j >= 0; j--) {
    const token = tokens[j]
    if (token.type !== 'word' || !['FROM', 'JOIN'].includes(token.value.toUpperCase())) continue
    const tail = tokens.slice(j + 1), isName = t => t && ['word', 'identifier'].includes(t.type)
    if (!tail.length) return { from: position, schema: null, prefix: '', padding: token.to === position ? ' ' : '' }
    if (tail.at(-1).to !== position) return null
    if (tail.length === 1 && isName(tail[0])) return { from: tail[0].from, schema: null, prefix: tail[0].value, padding: '' }
    if ((tail.length === 2 || tail.length === 3) && isName(tail[0]) && tail[1].value === '.' && (tail.length === 2 || isName(tail[2]))) {
      return { from: tail[0].from, schema: tail[0].value, prefix: tail[2]?.value || '', padding: '' }
    }
    return null
  }
  return null
}

export function tableCompletionOptions(tables, context) {
  const prefix = context.prefix.toLocaleLowerCase(), schema = context.schema?.toLocaleLowerCase()
  return tables.filter(table => schema != null
    ? table.schema.toLocaleLowerCase() === schema && table.name.toLocaleLowerCase().startsWith(prefix)
    : table.name.toLocaleLowerCase().startsWith(prefix) || `${table.schema}.${table.name}`.toLocaleLowerCase().startsWith(prefix)
  ).map(table => ({ label: `${table.schema}.${table.name}`, detail: table.schema, type: 'class', apply: context.padding + table.insertText }))
}
