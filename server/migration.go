package server

import (
	"log"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type Migration struct {
	Name string
	Sql  string
}

func (m Migration) apply() error {
	lines := strings.Split(m.Sql, ";\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			_, err := db.Exec(line)
			if err != nil {
				return errors.Wrap(err, "error in line: '"+line+"'")
			}
		}
	}
	return nil
}

type MigrationRow struct {
	Name string
	Time time.Time
}

func getFinishedMigrations() ([]MigrationRow, error) {
	db.MustExec("CREATE TABLE IF NOT EXISTS migrations (name TEXT, time TEXT)")
	var rows []MigrationRow
	if err := db.Select(&rows, "SELECT name FROM migrations"); err != nil {
		return nil, err
	}
	return rows, nil
}

func containsMigration(finished []MigrationRow, migration Migration) bool {
	for _, row := range finished {
		if row.Name == migration.Name {
			return true
		}
	}
	return false
}

func saveMigration(migration Migration) {
	db.MustExec("INSERT INTO migrations (name, time) VALUES ($1, $2)", migration.Name, time.Now())
}

func Migrate(migrations []Migration) {
	finished, err := getFinishedMigrations()
	if err != nil {
		log.Fatalln("could not read existing migrations", err)
	}

	for _, migration := range migrations {
		if containsMigration(finished, migration) {
			continue
		}
		log.Println("apply migration", migration.Name)
		if err := migration.apply(); err != nil {
			log.Fatalln("could not apply migration: '"+migration.Name+"'", err)
		}
		saveMigration(migration)
	}
}
