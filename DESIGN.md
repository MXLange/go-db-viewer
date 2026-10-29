# Go DB Viewer

## Contexto e direção

Ferramenta desktop para desenvolvedores e administradores de bancos. A ação principal é conectar, explorar e executar comandos.

Referência: `../avocadoterm`. Preservar Nunito e destaque verde-água. O tema escuro mantém fundo azul-escuro; o tema claro usa superfícies neutras.

Direção escolhida: workspace técnico compacto. Adjetivos: técnico, compacto, preciso e consistente.

- Técnico: editor monoespaçado e tipos visíveis nas colunas.
- Compacto: navegação persistente e painéis redimensionáveis.
- Preciso: conexão identificada em cada aba e resultados sem perda numérica.
- Consistente: componentes comuns para todos os bancos.

Essência: conexão, comando, resultado. Elemento característico: contexto da conexão acima do editor e marcadores SQL/JSON nas abas.

## Tipografia e tokens

Nunito local, licença OFL, para títulos e controles. Monoespaçada do sistema para editor, valores, tipos e indicadores.

Base: 14 px. Escala aproximada 1,2: 10, 12, 14, 17, 20 e 24 px. Título vazio: 40–64 px.

Tokens CSS em `frontend/src/style.css`. Cada cor possui fallback hexadecimal e declaração OKLCH.

| Papel | Token | Hexadecimal | OKLCH |
|---|---|---|---|
| Fundo | `--bg` | #0f172a | 20.8% .042 265.8 |
| Superfície | `--surface` | #151c2b | 22.8% .031 264 |
| Controle | `--raised` | #202a3c | 28.4% .036 262 |
| Texto | `--fg` | #e5e7eb | 92.8% .006 264.5 |
| Secundário | `--muted` | #a3b0c2 | 75% .03 257 |
| Borda | `--border` | #334155 | 37.2% .044 257.3 |
| Destaque | `--accent` | #14b8a6 | 70.4% .14 182.5 |
| Texto no destaque | `--accent-fg` | #05201d | 22% .035 181 |
| Seleção | `--selection` | #23404a | 35% .04 222 |
| Sucesso | `--success` | #5eead4 | 85.5% .138 181 |
| Aviso | `--warning` | #fde68a | 92.4% .12 95.7 |
| Erro | `--error` | #fca5a5 | 80.8% .114 19.5 |

Espaçamento base: 4 px. Grupos usam 8–12 px; separação de áreas usa 20–28 px.
Raios: 4 px nos controles; 8 px nos dialogs. Superfícies usam bordas, sem sombras.

## Composição e componentes

Barra lateral de 264 px, reduzida para 230 px em janelas menores. Janela mínima: 900 × 620.
Editor ocupa inicialmente 43% do painel. Separador aceita pointer e setas do teclado.

Botões primários usam verde-água; secundários usam borda. Ações auxiliares usam texto.
O editor não apresenta modo manual de execução. O backend identifica consultas e comandos sem retorno.
Linhas vazias separam os blocos. Ctrl+Enter executa somente o bloco do cursor quando o editor tem foco.
O botão Executar preserva o foco do editor e segue a mesma regra. Linhas separadoras não executam comandos.
Ctrl+Espaço abre o catálogo contextual após FROM e JOIN. A lista mostra schema e nome, usando as cores do editor.
As setas selecionam uma sugestão; Enter e Tab inserem; Esc fecha a lista.
Abas restauradas ficam editáveis offline. A ação Vincular conexão aparece no contexto da aba desconectada.
A barra inferior indica o salvamento do estado das abas. O editor indica o salvamento manual do script.
A biblioteca apresenta lista compacta, busca por nome e filtro por conexão. Fechar uma aba mantém o script salvo.
Dialogs nativos solicitam o nome no primeiro salvamento e confirmam ações sobre alterações pendentes.
Incluir estados hover, foco, desabilitado, carregamento, sucesso e erro.
Inputs possuem labels. O erro mantém os valores preenchidos. Dialog nativo contém o foco e devolve o foco ao fechar.

Tabelas usam texto à esquerda, números à direita e números tabulares. Tipos ficam abaixo dos nomes.
Resultados MongoDB usam Extended JSON canônico. Nunca usar dados fictícios como resultados reais.

## Movimento, acessibilidade e imagens

Sem imagens decorativas. Símbolos tipográficos pequenos apoiam labels textuais; não carregam significado isoladamente.
Transições de controles: 120 ms, respeitando `prefers-reduced-motion`. Sem animações de layout.
Foco de 2 px em verde-água. Alvos mínimos de 24 px; controles principais têm 34–42 px.
Estados usam texto além da cor. Mensagens usam `status` ou `alert` conforme a urgência.

## Auditoria

2026-10-29: revisão de código concluída para hierarquia, ausência de dados fictícios, labels, foco e estados.
Autocomplete e status de salvamento usam os tokens existentes. A validação automatizada da interface foi bloqueada pelo sandbox.
Validação visual, contraste calculado e uso com tecnologias assistivas dependem do ambiente de teste.
Não declarar conformidade WCAG sem essa validação.

## Histórico

2026-10-29: autocomplete contextual de tabelas e workspace persistente, com restauração sem conexão automática.

2026-10-29: removido o modo manual; execução automática do bloco do cursor, separado por linhas vazias.

2026-10-29: sistema inicial baseado no AvocadoTerm, adaptado ao fluxo conexão → comando → resultado.

2026-10-29: sugestões de colunas por alias e por contexto em WHERE/ON.
Ctrl+clique abre uma aba de propriedades com colunas, chaves e índices.
A aba usa tabelas compactas e os tokens existentes. Cada seção informa metadados indisponíveis ou não suportados.
Abas de propriedades não apresentam ações de execução ou paginação. Essas abas não fazem parte do workspace salvo.


2026-10-29: biblioteca de scripts nomeados por conexão, com salvamento manual por botão e Ctrl+S.
A direção técnica e compacta mantém Nunito, monoespaçada, escala 1,2 e a paleta existente.
O contexto da conexão e os marcadores SQL/JSON continuam identificando cada aba.
A lista usa linhas com nome, conexão e data. Ações auxiliares usam botões de texto.
Estados de gravação incluem texto e marcador na aba. Dialogs mantêm labels e erros junto aos controles.
Auditoria de código: sem novas imagens, cores ou sombras; novos controles usam os tokens existentes.
A auditoria visual e o contraste calculado permanecem pendentes: o sandbox bloqueou a inicialização do Chrome.


## Temas e configurações

A navegação principal apresenta Workspace e Configurações acima do conteúdo. A troca preserva as abas abertas.
O tema padrão segue o sistema. Claro e Escuro são escolhas persistentes em `settings.json`.
O tema claro mantém Nunito, monoespaçada, escala 1,2 e destaque verde-água. Superfícies claras usam texto azul-escuro.
Ambos os temas usam os mesmos tokens semânticos. `color-scheme` acompanha o tema efetivo nos controles nativos.

| Papel | Token | Claro: fallback | Claro: OKLCH |
|---|---|---|---|
| Fundo | `--bg` | #f6f8fa | 97.8% .004 248 |
| Superfície | `--surface` | #ffffff | 100% 0 0 |
| Controle | `--raised` | #eaf0f3 | 95% .009 232 |
| Texto | `--fg` | #20303a | 30% .024 239 |
| Secundário | `--muted` | #536774 | 50% .027 237 |
| Borda | `--border` | #81949f | 65% .025 237 |
| Destaque | `--accent` | #087e73 | 53% .09 183 |
| Texto no destaque | `--accent-fg` | #ffffff | 100% 0 0 |
| Seleção | `--selection` | #c5dfdf | 88% .028 195 |
| Ocorrência correspondente | `--selection-match` | #e1eeee | 94% .015 195 |
| Sucesso | `--success` | #066c5f | 47% .08 179 |
| Aviso | `--warning` | #815400 | 48% .1 74 |
| Erro | `--error` | #b42335 | 49% .17 19 |

No tema escuro, a seleção usa #23404a / OKLCH 35% .04 222.
Ocorrências correspondentes usam #1b333d / OKLCH 30% .03 222.
O CodeMirror configura a camada de seleção com precedência suficiente para impedir o destaque claro padrão.
A troca de tema reconfigura somente o tema do editor, preservando documento e seleção.

O rodapé dos resultados mostra quantidade/limite e permite alterar o máximo da aba.
Estados incluem limite padrão, substituição temporária, valor inválido, tamanho excedido e alteração pendente da próxima execução.
A tela de configurações apresenta labels, validação local, status de gravação e ação para repetir após erro.
Não há execução automática ao editar limites.

2026-10-29: revisão dos tokens e estados realizada em código. Contrastes teóricos dos tokens OKLCH: texto claro 12,75:1; secundário claro 5,96:1; seleção clara 9,56:1; botão primário claro 5,04:1; seleção escura 9,03:1.
Esses cálculos não substituem a validação do resultado renderizado.
A execução visual no Chrome foi bloqueada pelo sandbox. Não declarar conformidade de acessibilidade com base apenas na revisão estática.

2026-10-29: corrigida a seleção invisível na linha ativa do editor.
O fundo da linha ativa usa a cor da superfície com 35% de opacidade. O número da linha mantém fundo opaco.
Isso preserva a seleção desenhada pelo CodeMirror atrás do conteúdo, sem alterar o texto ou sua opacidade.
A regressão visual compara pixels do mesmo trecho selecionado e sem seleção; verificar somente a cor CSS era insuficiente.
Antes da correção, os cinco pontos amostrados tinham diferença RGB zero, apesar de SQL estar selecionado.
Depois da correção, passaram dez cenários visuais: teclado, mouse, múltiplas linhas, Ctrl+A e perda de foco nos dois temas.
A suíte completa da interface também passou após a liberação do ambiente de teste.
