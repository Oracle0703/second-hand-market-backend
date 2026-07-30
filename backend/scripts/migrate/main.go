package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/databasecmd"
)

const maxMigrationBytes = 1 << 20

type migrationSpec struct {
	ID      string
	Sources []migrationSource
}

type migrationSource struct {
	FileName string
	SHA256   string
}

var migrationCatalog = map[string]migrationSpec{
	"0001_init": {
		ID: "0001_init",
		Sources: []migrationSource{{
			FileName: "0001_init.up.sql",
			SHA256:   "78cc0be7c571e7ff2b14d283cbf03571f73d947474e726a0d07fc83f7d14dd92",
		}},
	},
	"0002_buyer_domain": {
		ID: "0002_buyer_domain",
		Sources: []migrationSource{{
			FileName: "0002_buyer_domain.up.sql",
			SHA256:   "c1c59f570df99a0799d9d6fedffe54dca2fad22da32b2ecdc1da7e50cc8480ed",
		}},
	},
	"0003_buyer_auth_provider": {
		ID: "0003_buyer_auth_provider",
		Sources: []migrationSource{{
			FileName: "0003_buyer_auth_provider.up.sql",
			SHA256:   "36fcc18878f9248f7d1e9bcce47f4cb726a8371048df10899e5ed082e76de036",
		}},
	},
	"0004_merchant_multi_stock": {
		ID: "0004_merchant_multi_stock",
		Sources: []migrationSource{
			{
				FileName: "0004_merchant_multi_stock.preflight.sql",
				SHA256:   "9b00fd6d32ef8e73d74fedbad154d99a584ebc5ef292d849b8776fddedf95865",
			},
			{
				FileName: "0004_merchant_multi_stock.up.sql",
				SHA256:   "ec2713616fb266ba653d3babfa738e896525e53cad6d87417dc8e629b092b3f2",
			},
			{
				FileName: "0004_merchant_multi_stock.postflight.sql",
				SHA256:   "c17ebf6c0595f15c9f7de8749b216cde2bc86fe57a3cd9b4984b8c6404288ae2",
			},
		},
	},
}

type migrationDependencies struct {
	loadConfig     func() (databasecmd.Config, error)
	openDatabase   func(databasecmd.Config) (*gorm.DB, error)
	closeDatabase  func(*gorm.DB)
	loadStatements func(migrationSpec) ([]string, error)
	execute        func(*gorm.DB, string) error
}

func main() {
	migrationID, err := run(os.Args[1:], defaultMigrationDependencies())
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	log.Printf("DATABASE_MIGRATION PASS migration=%s", migrationID)
}

func defaultMigrationDependencies() migrationDependencies {
	return migrationDependencies{
		loadConfig:    databasecmd.LoadConfig,
		openDatabase:  databasecmd.OpenDatabase,
		closeDatabase: databasecmd.CloseDatabase,
		loadStatements: func(spec migrationSpec) ([]string, error) {
			return loadMigrationStatementsFromDir("migrations", spec)
		},
		execute: func(db *gorm.DB, statement string) error {
			return db.Exec(statement).Error
		},
	}
}

func run(args []string, dependencies migrationDependencies) (string, error) {
	spec, err := parseMigrationSelection(args)
	if err != nil {
		return "", err
	}

	statements, err := dependencies.loadStatements(spec)
	if err != nil || len(statements) == 0 {
		return "", errors.New("DATABASE_MIGRATION source validation failed")
	}

	databaseConfig, err := dependencies.loadConfig()
	if err != nil {
		return "", err
	}
	if databaseConfig.Driver != "mysql" {
		return "", errors.New("DB_DRIVER must be mysql for SQL migrations")
	}

	db, err := dependencies.openDatabase(databaseConfig)
	if err != nil {
		return "", err
	}
	defer dependencies.closeDatabase(db)

	for _, statement := range statements {
		if err := dependencies.execute(db, statement); err != nil {
			return "", errors.New("DATABASE_MIGRATION failed migration=" + spec.ID)
		}
	}
	return spec.ID, nil
}

func parseMigrationSelection(args []string) (migrationSpec, error) {
	if len(args) == 0 {
		return migrationSpec{}, errors.New("MIGRATION_SELECTION is required")
	}

	var selection string
	switch {
	case len(args) == 1 && strings.HasPrefix(args[0], "--migration="):
		selection = strings.TrimPrefix(args[0], "--migration=")
	case len(args) == 2 && args[0] == "--migration":
		selection = args[1]
	default:
		return migrationSpec{}, errors.New("MIGRATION_ARGUMENTS are invalid")
	}
	if selection == "" {
		return migrationSpec{}, errors.New("MIGRATION_SELECTION is required")
	}
	if strings.TrimSpace(selection) != selection {
		return migrationSpec{}, errors.New("MIGRATION_SELECTION is invalid")
	}

	spec, ok := migrationCatalog[selection]
	if !ok {
		return migrationSpec{}, errors.New("MIGRATION_SELECTION is invalid")
	}
	return spec, nil
}

func loadMigrationStatementsFromDir(directory string, spec migrationSpec) ([]string, error) {
	if len(spec.Sources) == 0 {
		return nil, errors.New("migration source is invalid")
	}

	var statements []string
	for _, source := range spec.Sources {
		sourceStatements, err := loadMigrationSourceFromDir(directory, source)
		if err != nil {
			return nil, err
		}
		statements = append(statements, sourceStatements...)
	}
	return statements, nil
}

func loadMigrationSourceFromDir(directory string, source migrationSource) ([]string, error) {
	path := filepath.Join(directory, source.FileName)
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("migration source is invalid")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("migration source is invalid")
	}
	defer file.Close()

	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, errors.New("migration source is invalid")
	}

	data, err := io.ReadAll(io.LimitReader(file, maxMigrationBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxMigrationBytes {
		return nil, errors.New("migration source is invalid")
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if bytes.ContainsRune(data, '\r') {
		return nil, errors.New("migration source is invalid")
	}

	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != source.SHA256 {
		return nil, errors.New("migration source is invalid")
	}
	return splitSQLStatements(string(data))
}

func splitSQLStatements(source string) ([]string, error) {
	var (
		statements   []string
		current      strings.Builder
		quote        byte
		lineComment  bool
		blockComment bool
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
		case character == ';':
			appendStatement()
		default:
			current.WriteByte(character)
		}
	}

	if quote != 0 || blockComment {
		return nil, errors.New("migration SQL is invalid")
	}
	appendStatement()
	if len(statements) == 0 {
		return nil, errors.New("migration SQL is empty")
	}
	return statements, nil
}
