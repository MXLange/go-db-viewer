// Linhas vazias delimitam blocos. A seleção não altera o cursor principal.
export function commandAtCursor(document, head, language = 'sql') {
  if (!Number.isInteger(head) || head < 0 || head > document.length) return null
  const lines = document.split('\n')
  const separators = language === 'sql' ? sqlSeparators(lines) : lines.map(line => !line.trim())
  let line = 0, offset = 0
  while (line < lines.length - 1 && head > offset + lines[line].length) {
    offset += lines[line].length + 1
    line++
  }
  if (separators[line]) return null
  let first = line, last = line
  while (first > 0 && !separators[first - 1]) first--
  while (last + 1 < lines.length && !separators[last + 1]) last++
  const text = lines.slice(first, last + 1).join('\n').trim()
  if (language === 'sql' && !hasSQLCommand(text)) return null
  return text
}

// Uma linha dentro de um literal ou comentário não inicia outro comando.
function sqlSeparators(lines) {
  const separators = []
  const nameCharacter = /[\p{L}\p{N}_$#]/u
  let quote = '', dollar = '', oracle = '', escaped = false, comment = 0
  for (const line of lines) {
    separators.push(!line.trim() && !quote && !dollar && !oracle && !comment)
    for (let i = 0; i < line.length; i++) {
      if (dollar) {
        if (line.startsWith(dollar, i)) { i += dollar.length - 1; dollar = '' }
      } else if (oracle) {
        if (line.startsWith(oracle, i)) { i += oracle.length - 1; oracle = '' }
      } else if (quote) {
        if (escaped && line[i] === '\\') i++
        else if (line[i] === quote) {
          if (line[i + 1] === quote) i++
          else quote = ''
        }
      } else if (comment) {
        if (line.startsWith('/*', i)) { comment++; i++ }
        else if (line.startsWith('*/', i)) { comment--; i++ }
      } else if (line.startsWith('--', i)) break
      else if (line.startsWith('/*', i)) { comment++; i++ }
      else if ((line[i] === 'q' || line[i] === 'Q') && !nameCharacter.test(line[i - 1] || '') && line[i + 1] === "'" && line[i + 2]) {
        const open = line[i + 2]
        oracle = ({ '[': ']', '{': '}', '(': ')', '<': '>' }[open] || open) + "'"
        i += 2
      } else if (line[i] === '$' && !nameCharacter.test(line[i - 1] || '')) {
        const tag = /^\$[a-zA-Z0-9_]*\$/.exec(line.slice(i))?.[0]
        if (tag) { dollar = tag; i += tag.length - 1 }
      } else if (["'", '"', '[', '`'].includes(line[i])) {
        quote = line[i] === '[' ? ']' : line[i]
        escaped = quote === "'" && /e/i.test(line[i - 1] || '') && !nameCharacter.test(line[i - 2] || '')
      }
    }
  }
  return separators
}

function hasSQLCommand(text) {
  let rest = text.trimStart()
  while (rest) {
    if (rest.startsWith('--')) {
      const end = rest.indexOf('\n')
      if (end < 0) return false
      rest = rest.slice(end + 1).trimStart()
    } else if (rest.startsWith('/*')) {
      let depth = 1, end = 2
      while (end < rest.length && depth) {
        if (rest.startsWith('/*', end)) { depth++; end += 2 }
        else if (rest.startsWith('*/', end)) { depth--; end += 2 }
        else end++
      }
      if (depth) return false
      rest = rest.slice(end).trimStart()
    } else if (rest.startsWith(';')) rest = rest.slice(1).trimStart()
    else return true
  }
  return false
}
