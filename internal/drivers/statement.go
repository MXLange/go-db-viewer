package drivers

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"go-db-viewer/internal/core"
)

type sqlToken struct {
	value      string
	depth      int
	word       bool
	identifier bool
}

type sqlStatement struct {
	text        string
	returnsRows bool
}

const multipleStatements = core.SafeFault("Separe os comandos por linhas vazias. Cada bloco deve conter somente um comando SQL.")

// scanStatement mantém literais e identificadores fora da análise de keywords.
func scanStatement(text string) ([]sqlToken, int, error) {
	var tokens []sqlToken
	depth, terminator := 0, -1
	add := func(value string, word, identifier bool) {
		tokens = append(tokens, sqlToken{value: value, depth: depth, word: word, identifier: identifier})
	}
	for i := 0; i < len(text); {
		r, width := utf8.DecodeRuneInString(text[i:])
		if unicode.IsSpace(r) {
			i += width
			continue
		}
		if strings.HasPrefix(text[i:], "--") {
			j := strings.IndexByte(text[i:], '\n')
			if j < 0 {
				break
			}
			i += j + 1
			continue
		}
		if strings.HasPrefix(text[i:], "/*") {
			i += 2
			nesting := 1
			for i < len(text) && nesting > 0 {
				if strings.HasPrefix(text[i:], "/*") {
					nesting++
					i += 2
				} else if strings.HasPrefix(text[i:], "*/") {
					nesting--
					i += 2
				} else {
					i++
				}
			}
			if nesting != 0 {
				return nil, -1, core.SafeFault("Comentário SQL não terminado.")
			}
			continue
		}
		if terminator >= 0 {
			return nil, -1, multipleStatements
		}
		if text[i] == ';' {
			if depth != 0 {
				return nil, -1, core.SafeFault("Ponto e vírgula fora de um literal SQL.")
			}
			terminator = i
			i++
			continue
		}
		// Literais Oracle q'[texto]' podem conter aspas e ponto e vírgula.
		if (text[i] == 'q' || text[i] == 'Q') && i+2 < len(text) && text[i+1] == '\'' {
			close := text[i+2]
			switch close {
			case '[':
				close = ']'
			case '{':
				close = '}'
			case '(':
				close = ')'
			case '<':
				close = '>'
			}
			j := strings.Index(text[i+3:], string([]byte{close, '\''}))
			if j < 0 {
				return nil, -1, core.SafeFault("Literal SQL não terminado.")
			}
			i += 3 + j + 2
			add("literal", false, false)
			continue
		}
		if text[i] == '\'' || text[i] == '"' || text[i] == '[' || text[i] == '`' {
			quote := text[i]
			close := quote
			if quote == '[' {
				close = ']'
			}
			escaped := quote == '\'' && i > 0 && (text[i-1] == 'E' || text[i-1] == 'e') && (i == 1 || !isSQLName(rune(text[i-2])))
			i++
			closed := false
			for i < len(text) {
				if escaped && text[i] == '\\' {
					i += 2
					continue
				}
				if text[i] == close {
					i++
					if i < len(text) && text[i] == close {
						i++
						continue
					}
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, -1, core.SafeFault("Literal ou identificador SQL não terminado.")
			}
			add("quoted", false, quote != '\'')
			continue
		}
		if text[i] == '$' {
			j := i + 1
			for j < len(text) && (text[j] == '_' || text[j] >= 'a' && text[j] <= 'z' || text[j] >= 'A' && text[j] <= 'Z' || text[j] >= '0' && text[j] <= '9') {
				j++
			}
			if j < len(text) && text[j] == '$' {
				tag := text[i : j+1]
				end := strings.Index(text[j+1:], tag)
				if end < 0 {
					return nil, -1, core.SafeFault("Literal SQL não terminado.")
				}
				i = j + 1 + end + len(tag)
				add("literal", false, false)
				continue
			}
		}
		if unicode.IsLetter(r) || r == '_' {
			j := i + width
			for j < len(text) {
				next, n := utf8.DecodeRuneInString(text[j:])
				if !isSQLName(next) {
					break
				}
				j += n
			}
			add(strings.ToUpper(text[i:j]), true, true)
			i = j
			continue
		}
		switch text[i] {
		case '(':
			add("(", false, false)
			depth++
		case ')':
			depth--
			if depth < 0 {
				return nil, -1, core.SafeFault("Parênteses SQL não balanceados.")
			}
			add(")", false, false)
		default:
			add(string(text[i]), false, false)
		}
		i++
	}
	if depth != 0 {
		return nil, -1, core.SafeFault("Parênteses SQL não balanceados.")
	}
	if len(tokens) == 0 {
		return nil, -1, core.SafeFault("Informe um comando SQL.")
	}
	return tokens, terminator, nil
}

func isSQLName(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$' || r == '#'
}
func keyword(token sqlToken, word string) bool { return token.word && token.value == word }

func validateStatement(tokens []sqlToken) error {
	switch tokens[0].value {
	case "BEGIN", "COMMIT", "ROLLBACK", "SAVEPOINT", "RELEASE", "DECLARE":
		return core.SafeFault("Scripts, blocos e transações manuais não são aceitos nesta versão.")
	case "START":
		if len(tokens) > 1 && keyword(tokens[1], "TRANSACTION") {
			return core.SafeFault("Transações manuais não são aceitas nesta versão.")
		}
	case "SET":
		return core.SafeFault("Comandos SET não são aceitos nesta versão. Use opções na string de conexão.")
	}
	return nil
}

func singleStatement(text string) error {
	tokens, _, err := scanStatement(text)
	if err != nil {
		return err
	}
	return validateStatement(tokens)
}

// mainCommand pula as definições CTE sem interpretar seus comandos internos.
func mainCommand(tokens []sqlToken) (int, error) {
	if !keyword(tokens[0], "WITH") {
		return 0, nil
	}
	invalid := core.SafeFault("Estrutura WITH não suportada. Informe uma CTE seguida de SELECT, INSERT, UPDATE, DELETE ou MERGE.")
	i := 1
	if i < len(tokens) && keyword(tokens[i], "RECURSIVE") {
		i++
	}
	skipGroup := func(index int) int {
		if index >= len(tokens) || tokens[index].value != "(" {
			return -1
		}
		for j := index + 1; j < len(tokens); j++ {
			if tokens[j].value == ")" && tokens[j].depth == tokens[index].depth {
				return j + 1
			}
		}
		return -1
	}
	for {
		if i >= len(tokens) || !tokens[i].identifier {
			return 0, invalid
		}
		i++
		if i < len(tokens) && tokens[i].value == "(" {
			i = skipGroup(i)
			if i < 0 {
				return 0, invalid
			}
		}
		if i >= len(tokens) || !keyword(tokens[i], "AS") {
			return 0, invalid
		}
		i++
		if i < len(tokens) && keyword(tokens[i], "NOT") {
			i++
			if i >= len(tokens) || !keyword(tokens[i], "MATERIALIZED") {
				return 0, invalid
			}
			i++
		} else if i < len(tokens) && keyword(tokens[i], "MATERIALIZED") {
			i++
		}
		i = skipGroup(i)
		if i < 0 || i >= len(tokens) {
			return 0, invalid
		}
		if tokens[i].value != "," {
			return i, nil
		}
		i++
	}
}

func analyzeStatement(text string, kind core.Kind) (sqlStatement, error) {
	tokens, terminator, err := scanStatement(text)
	if err != nil {
		return sqlStatement{}, err
	}
	if err := validateStatement(tokens); err != nil {
		return sqlStatement{}, err
	}
	main, err := mainCommand(tokens)
	if err != nil {
		return sqlStatement{}, err
	}
	if !tokens[main].word {
		return sqlStatement{}, core.SafeFault("Tipo de comando SQL não suportado.")
	}
	statement := sqlStatement{text: text}
	if kind == core.Oracle && terminator >= 0 {
		statement.text = text[:terminator] + text[terminator+1:]
	}
	hasClause := func(word string, after int) int {
		for i := after; i < len(tokens); i++ {
			if tokens[i].depth == 0 && keyword(tokens[i], word) {
				return i
			}
		}
		return -1
	}
	switch tokens[main].value {
	case "SELECT":
		statement.returnsRows = hasClause("INTO", main+1) < 0
	case "VALUES", "TABLE", "SHOW", "EXPLAIN", "DESCRIBE", "DESC":
		statement.returnsRows = true
	case "INSERT", "UPDATE", "DELETE", "MERGE":
		if kind == core.Postgres {
			statement.returnsRows = hasClause("RETURNING", main+1) >= 0
		}
		if kind == core.SQLServer {
			output := hasClause("OUTPUT", main+1)
			statement.returnsRows = output >= 0 && hasClause("INTO", output+1) < 0
		}
	case "CREATE", "ALTER", "DROP", "TRUNCATE", "GRANT", "REVOKE", "COMMENT", "RENAME", "REFRESH", "VACUUM", "ANALYZE", "REINDEX", "CLUSTER":
	default:
		return sqlStatement{}, core.SafeFault("Tipo de comando SQL não suportado para execução automática.")
	}
	return statement, nil
}
