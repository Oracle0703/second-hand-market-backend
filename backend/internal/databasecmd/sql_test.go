package databasecmd

import (
	"strings"
	"testing"
)

func TestSplitSQLStatementsKeepsRoutineBodyAndQuotedDelimiters(t *testing.T) {
	statements, err := SplitSQLStatements(`-- ordinary statement
DROP PROCEDURE IF EXISTS example;
DELIMITER //
CREATE PROCEDURE example()
BEGIN
  SELECT 'semi; and // and DELIMITER ;';
  /* // is not a delimiter in a comment */
  SELECT 2;
END//
DELIMITER ;
CALL example();
DROP PROCEDURE example;
`)
	if err != nil || len(statements) != 4 {
		t.Fatalf("statements=%q, err=%v", statements, err)
	}
	if !strings.Contains(statements[1], "SELECT 2;") || !strings.HasSuffix(statements[1], "END") {
		t.Fatalf("routine body was split: %q", statements[1])
	}
	for _, source := range []string{
		"DELIMITER unsupported\nSELECT 1;",
		"DELIMITER //\nSELECT 1//",
		"SELECT 1\nDELIMITER //\n",
	} {
		if _, err := SplitSQLStatements(source); err == nil {
			t.Fatalf("accepted malformed SQL %q", source)
		}
	}
}
