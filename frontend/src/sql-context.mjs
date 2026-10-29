// O scanner mantém posições e ignora conteúdo de literais e comentários.
export function sqlTokens(source) {
  const tokens = [], name = /[\p{L}\p{N}_$#]/u
  let i = 0
  const push = (type, start, value = source.slice(start, i), closed = true) => tokens.push({ type, value, from: start, to: i, closed, raw: source.slice(start,i) })
  while (i < source.length) {
    const start = i, c = source[i]
    if (/\s/.test(c)) {
      while (i < source.length && /\s/.test(source[i])) i++
      if (/\n[\t \r]*\n/.test(source.slice(start, i))) push('barrier', start)
      continue
    }
    if (source.startsWith('--', i)) {
      i = source.indexOf('\n', i + 2); if (i < 0) i = source.length
      push('comment', start); continue
    }
    if (source.startsWith('/*', i)) {
      i += 2; let depth = 1
      while (i < source.length && depth) {
        if (source.startsWith('/*', i)) { depth++; i += 2 }
        else if (source.startsWith('*/', i)) { depth--; i += 2 }
        else i++
      }
      push('comment', start, '', !depth); continue
    }
    if ((c === 'q' || c === 'Q') && source[i + 1] === "'" && source[i + 2]) {
      const open = source[i + 2], close = ({ '[': ']', '{': '}', '(': ')', '<': '>' }[open] || open) + "'"
      const end = source.indexOf(close, i + 3); i = end < 0 ? source.length : end + close.length
      push('literal', start, '', end >= 0); continue
    }
    if (c === '$') {
      const tag = /^\$[a-zA-Z0-9_]*\$/.exec(source.slice(i))?.[0]
      if (tag) {
        const end = source.indexOf(tag, i + tag.length); i = end < 0 ? source.length : end + tag.length
        push('literal', start, '', end >= 0); continue
      }
    }
    if (["'", '"', '[', '`'].includes(c)) {
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
      push(c === "'" ? 'literal' : 'identifier', start, value, closed); continue
    }
    if (/[\p{L}_]/u.test(c)) {
      i++; while (i < source.length && name.test(source[i])) i++
      push('word', start); continue
    }
    if (/[0-9]/.test(c)) {
      i++; while (i < source.length && /[0-9.eE+-]/.test(source[i])) i++
      push('number', start); continue
    }
    i++; push(c === ';' ? 'barrier' : 'symbol', start)
  }
  return tokens
}
const isName = token => token && ['word', 'identifier'].includes(token.type)
const keyword = (token, value) => token?.type === 'word' && token.value.toUpperCase() === value
const reserved = new Set('WHERE JOIN LEFT RIGHT FULL INNER OUTER CROSS ON USING GROUP ORDER HAVING LIMIT OFFSET FETCH UNION INTERSECT EXCEPT RETURNING SET VALUES WINDOW FOR NATURAL CONNECT START QUALIFY WITH TABLESAMPLE'.split(' '))
const key = (token, dialect) => dialect === 'sqlserver' ? token.value.toUpperCase() : token.type === 'identifier' ? token.value : dialect === 'postgresql' ? token.value.toLowerCase() : token.value.toUpperCase()
const matches = (a, b, dialect) => key(a,dialect) === key(b,dialect)
const reference = (schema, name) => ({ schema: schema?.value || '', name: name.value, schemaQuoted: schema?.type === 'identifier', nameQuoted: name.type === 'identifier' })

function analyze(document, position, dialect = 'postgresql') {
  const all = sqlTokens(document)
  if (all.some(t => ['literal','comment','number','barrier'].includes(t.type) && t.from < position && (position < t.to || (position === t.to && (t.type === 'number' || !t.closed || t.type === 'comment' && document.slice(t.from,t.from+2)==='--'))))) return null
  const before = all.filter(t => t.type === 'barrier' && t.to <= position).at(-1)?.to || 0
  const after = all.find(t => t.type === 'barrier' && t.from >= position)?.from ?? document.length
  const tokens = all.filter(t => t.from >= before && t.to <= after && t.type !== 'comment' && t.type !== 'barrier')
  const root = { parent: null, tokens: [], from: before, to: after }, groups = [root], stack = [root]
  for (const token of tokens) {
    if (token.value === '(' && token.type === 'symbol') {
      const parent = stack.at(-1), group = { parent, tokens: [], from: token.to, to: after, open: token }
      parent.tokens.push({ ...token, group }); groups.push(group); stack.push(group)
    } else if (token.value === ')' && token.type === 'symbol' && stack.length > 1) {
      const group = stack.pop(); group.to = token.from; group.close = token
    } else stack.at(-1).tokens.push(token)
  }
  const isQuery = g => g === root || g.tokens.some(t => keyword(t,'SELECT') || keyword(t,'WITH'))
  let current = groups.filter(g => g.from <= position && g.to >= position && isQuery(g)).at(-1) || root
  // Cada braço de UNION tem seu próprio conjunto de relações.
  const visibleTokens = g => {
    const cuts = g.tokens.filter(t => ['UNION','INTERSECT','EXCEPT'].some(k => keyword(t,k)))
    const start = cuts.filter(t => t.to <= position).at(-1)?.to ?? g.from
    const end = cuts.find(t => t.from >= position)?.from ?? g.to
    return g.tokens.filter(t => t.from >= start && t.to <= end)
  }
  const ctes = []
  for (const g of groups.filter(g => isQuery(g) && g.from <= current.from && g.to >= current.to)) {
    let inWith = false
    for (let i=0;i<g.tokens.length;i++) {
      if (keyword(g.tokens[i],'WITH')) inWith = true
      if (inWith && keyword(g.tokens[i],'SELECT')) break
      if (inWith && isName(g.tokens[i]) && (keyword(g.tokens[i+1],'AS') || g.tokens[i+1]?.group && keyword(g.tokens[i+2],'AS'))) ctes.push(g.tokens[i])
    }
  }
  const relationsFor = g => {
    const ts = visibleTokens(g), relations = []; let inFrom = false, expect = false
    for (let i=0;i<ts.length;i++) {
      const t=ts[i]
      if (expect && (keyword(t,'ONLY') || keyword(t,'LATERAL'))) continue
      if (keyword(t,'FROM') || keyword(t,'JOIN')) { inFrom=true; expect=true; continue }
      if (t.type==='word' && reserved.has(t.value.toUpperCase())) { if (!keyword(t,'JOIN')) inFrom=false; expect=false; continue }
      if (t.value===',' && inFrom) {expect=true;continue}
      if (!expect) continue
      expect=false
      let schema, table=t, physical=true, locations=[]
      if (t.group) {physical=false;table=null}
      else if (isName(t)) {
        locations.push(t)
        if (ts[i+1]?.value==='.' && isName(ts[i+2])) {schema=t;table=ts[i+2];locations.push(table);i+=2}
        if (ts[i+1]?.value==='.' || ts[i+1]?.group) { physical=false; if (ts[i+1]?.group) i++ }
        if (!schema && ctes.some(c=>matches(c,table,dialect))) physical=false
      } else continue
      if (keyword(ts[i+1],'AS')) i++
      const candidate=ts[i+1]
      const alias=isName(candidate) && !(candidate.type==='word' && reserved.has(candidate.value.toUpperCase())) ? candidate : null
      if(alias){i++;locations.push(alias)}
      relations.push({ reference: physical && table ? reference(schema,table) : null, qualifier:alias||table, alias, table, locations, qualifierText: alias?.raw || (schema ? schema.raw + '.' : '') + (table?.raw || '') })
    }
    return relations
  }
  const canCorrelate = group => {
    if (!group.parent) return false
    const parent = group.parent, index = parent.tokens.findIndex(t => t.group === group)
    const previous = parent.tokens[index - 1]
    if (['AS','MATERIALIZED','FROM','JOIN'].some(k => keyword(previous,k))) return false
    if (previous?.value === ',') {
      const clause = parent.tokens.slice(0,index).filter(t => ['FROM','JOIN','WHERE','ON','SELECT'].some(k => keyword(t,k))).at(-1)
      if (keyword(clause,'FROM') || keyword(clause,'JOIN')) return false
    }
    return true
  }
  const relations=[], used=new Set()
  let g=current
  while(g){
    if(isQuery(g)) for(const r of relationsFor(g)) {
      if(!r.qualifier)continue
      const k=key(r.qualifier,dialect); if(!used.has(k)){relations.push(r)}
    }
    if(isQuery(g)) for(const r of relationsFor(g)) if(r.qualifier)used.add(key(r.qualifier,dialect))
    if (!canCorrelate(g)) break
    g=g.parent
  }
  return { tokens, current, relations, localTokens:visibleTokens(current) }
}

export function columnCompletionContext(document, position, explicit = true, dialect = 'postgresql') {
  const scope=analyze(document,position,dialect);if(!scope)return null
  const preceding=scope.tokens.filter(t=>t.from<position).map(t=>t.to>position && isName(t) ? {...t,to:position,value:document.slice(t.from+(t.type==='identifier'?1:0),position)} : t).filter(t=>t.to<=position)
  let last=preceding.at(-1), prefix='', from=position
  if(isName(last) && last.to===position){prefix=last.value;from=last.from;preceding.pop();last=preceding.at(-1)}
  let relations=scope.relations.filter(r=>r.reference)
  if(last?.value==='.' && last.to<=from){
    const qualifier=preceding.at(-2)
    if(!isName(qualifier))return null
    let found=scope.relations.filter(r=>matches(r.qualifier,qualifier,dialect) || !r.alias && r.table && matches(r.table,qualifier,dialect))
    if (preceding.at(-3)?.value === '.') {
      const schema = preceding.at(-4)
      if (!isName(schema) || preceding.at(-5)?.value === '.') return null
      found = found.filter(r => !r.alias && r.reference?.schema && matches({type:r.reference.schemaQuoted?'identifier':'word',value:r.reference.schema},schema,dialect))
    }
    if(found.length!==1 || !found[0].reference)return null
    return {from,prefix,relations:found,qualified:true,dialect}
  }
  if(!explicit)return null
  const clause=scope.localTokens.filter(t=>t.to<=from && t.type==='word' && ['SELECT','FROM','WHERE','ON','JOIN','GROUP','ORDER','HAVING','LIMIT','UNION'].includes(t.value.toUpperCase())).at(-1)?.value.toUpperCase()
  if(!['WHERE','ON','HAVING'].includes(clause) || !relations.length)return null
  return {from,prefix,relations,qualified:false,dialect}
}

export function columnCompletionOptions(entries, context) {
  const count=new Map(), columnKey=name=>context.dialect==='sqlserver'?name.toUpperCase():name
  for(const {columns} of entries)for(const c of columns)count.set(columnKey(c.name),(count.get(columnKey(c.name))||0)+1)
  const quote=t=>t.type==='identifier' ? t.raw : t.value
  const options=[]
  for(const {relation,columns} of entries)for(const column of columns){
    if(!column.name.toLocaleLowerCase().startsWith(context.prefix.toLocaleLowerCase()))continue
    const qualifier=relation.qualifierText || quote(relation.qualifier), name=column.insertText||'"'+column.name.replaceAll('"','""')+'"'
    const ambiguous=count.get(columnKey(column.name))>1
    options.push({label:context.qualified||!ambiguous?column.name:`${qualifier}.${column.name}`,detail:`${qualifier} · ${column.type}`,type:'property',apply:!context.qualified&&ambiguous?`${qualifier}.${name}`:name})
  }
  return options
}

export function tableReferenceAt(document, position, dialect = 'postgresql') {
  const scope=analyze(document,position,dialect);if(!scope)return null
  const token=scope.tokens.find(t=>isName(t)&&t.from<=position&&t.to>=position)
  if(!token)return null
  const declared=scope.relations.filter(r=>r.locations.some(t=>t.from===token.from))
  if(declared.length===1)return declared[0].reference
  const index=scope.tokens.indexOf(token)
  if(scope.tokens[index+1]?.value!=='.')return null
  const found=scope.relations.filter(r=>matches(r.qualifier,token,dialect))
  return found.length===1?found[0].reference:null
}
