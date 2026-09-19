package databasecmd

import (
	"errors"
	"strings"
)

// SplitSQLStatements handles quoted SQL and MySQL client delimiter directives.
func SplitSQLStatements(source string) ([]string, error) {
	var (
		statements   []string
		current      strings.Builder
		quote        byte
		lineComment  bool
		blockComment bool
		delimiter    = ";"
	)

	appendStatement := func() {
		statement := strings.TrimSpace(current.String())
		current.Reset()
		if statement != "" {
			statements = append(statements, statement)
		}
	}

	for i := 0; i < len(source); i++ {
		character := source[i]

		if lineComment {
			if character == '\n' {
				lineComment = false
				current.WriteByte('\n')
			}
			continue
		}
		if blockComment {
			if character == '*' && i+1 < len(source) && source[i+1] == '/' {
				blockComment = false
				current.WriteByte(' ')
				i++
			}
			continue
		}
		if quote != 0 {
			current.WriteByte(character)
			if character == '\\' && quote != '`' && i+1 < len(source) {
				i++
				current.WriteByte(source[i])
				continue
			}
			if character == quote {
				if i+1 < len(source) && source[i+1] == quote {
					i++
					current.WriteByte(source[i])
					continue
				}
				quote = 0
			}
			continue
		}

		// DELIMITER is a mysql client directive, never sent to the server.
		if (i == 0 || source[i-1] == '\n') && strings.HasPrefix(source[i:], "DELIMITER ") {
			if strings.TrimSpace(current.String()) != "" {
				return nil, errors.New("delimiter inside unfinished statement")
			}
			end := strings.IndexByte(source[i:], '\n')
			if end < 0 {
				end = len(source) - i
			}
			delimiter = strings.TrimSpace(strings.TrimPrefix(source[i:i+end], "DELIMITER "))
			if delimiter != ";" && delimiter != "//" {
				return nil, errors.New("unsupported SQL delimiter")
			}
			i += end - 1
			continue
		}
		if strings.HasPrefix(source[i:], delimiter) {
			appendStatement()
			i += len(delimiter) - 1
			continue
		}

		switch {
		case character == '-' && i+1 < len(source) && source[i+1] == '-':
			lineComment = true
			current.WriteByte(' ')
			i++
		case character == '#':
			lineComment = true
			current.WriteByte(' ')
		case character == '/' && i+1 < len(source) && source[i+1] == '*':
			blockComment = true
			current.WriteByte(' ')
			i++
		case character == '\'' || character == '"' || character == '`':
			quote = character
			current.WriteByte(character)
		default:
			current.WriteByte(character)
		}
	}

	if quote != 0 || blockComment || delimiter != ";" {
		return nil, errors.New("migration SQL is invalid")
	}
	appendStatement()
	if len(statements) == 0 {
		return nil, errors.New("migration SQL is empty")
	}
	return statements, nil
}
