package server

import (
	"database/sql"
	"encoding/json"
	"github.com/pkg/errors"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
)

var db *sqlx.DB

type PackageRow struct {
	Name          string
	Info          string
	LatestVersion string `db:"latest_version"`
}

func DbGetPackage(name string) (*PackageInfo, error) {
	var row PackageRow
	if err := db.Get(&row, "SELECT info FROM packages WHERE name = $1", name); err != nil {
		return nil, err
	}
	var packageInfo PackageInfo
	if err := json.Unmarshal([]byte(row.Info), &packageInfo); err != nil {
		return nil, err
	}
	return &packageInfo, nil
}

func DbGetPackageLatestVersion(name string) (string, error) {
	var row PackageRow
	if err := db.Get(&row, "SELECT latest_version FROM packages WHERE name = $1", name); err != nil {
		return "", err
	}
	return row.LatestVersion, nil
}

func DbPutPackage(name string, packageInfo *PackageInfo, expireTime time.Time) error {
	bytes, err := json.Marshal(packageInfo)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO packages (name, info, latest_version, create_time, expire_time) VALUES ($1, $2, $3, $4, $5)",
		name, bytes, packageInfo.DistTags.Latest, time.Now(), expireTime)
	return err
}

type VersionRow struct {
	Name    string
	Version string
	Content string
}

func DbGetVersion(name string, versionRaw string) (*Version, error) {
	var row VersionRow
	if err := db.Get(&row, "SELECT content FROM versions WHERE name = $1 AND version = $2", name, versionRaw); err != nil {
		return nil, err
	}
	var version Version
	if err := json.Unmarshal([]byte(row.Content), &version); err != nil {
		return nil, err
	}
	return &version, nil
}

func DbPutVersion(name string, versionRaw string, version *Version, expireTime time.Time) error {
	bytes, err := json.Marshal(version)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO versions (name, version, content, create_time, expire_time) VALUES ($1, $2, $3, $4, $5)",
		name, versionRaw, bytes, time.Now(), expireTime)
	return err
}

type FileRow struct {
	Id      string
	Content string
}

func DbGetFile(id string) (*Version, error) {
	var row FileRow
	if err := db.Get(&row, "SELECT content FROM files WHERE id = $1", id); err != nil {
		return nil, err
	}
	var version Version
	if err := json.Unmarshal([]byte(row.Content), &version); err != nil {
		return nil, err
	}
	return &version, nil
}

func DbPutFile(id string, version *Version) error {
	bytes, err := json.Marshal(version)
	if err != nil {
		return err
	}
	// TODO transaction
	if _, err = DbGetFile(id); err != nil {
		if err == sql.ErrNoRows {
			_, err = db.Exec("INSERT INTO files (id, content, create_time) VALUES ($1, $2, $3)", id, bytes, time.Now())
		}
	} else {
		_, err = db.Exec("UPDATE files SET content = $2 WHERE id = $1", id, bytes)
	}
	return err
}

type VulnerabilityRow struct {
	Id              string
	Name            string
	Title           string
	PublicationTime string `db:"publication_time"`
	Semver          []byte
	Severity        string
}

func DbLastVulnerability() (*Vulnerability, error) {
	var row VulnerabilityRow
	if err := db.Get(&row, "SELECT id, publication_time FROM vulnerabilities ORDER BY publication_time DESC LIMIT 1"); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		} else {
			return nil, err
		}
	}
	publicationTime, err := time.Parse(time.RFC3339Nano, row.PublicationTime)
	if err != nil {
		return nil, errors.Wrapf(err, "could not parse time %s", row.PublicationTime)
	}
	return &Vulnerability{Id: row.Id, PublicationTime: publicationTime}, nil
}

func DbPutVulnerability(vulnerability Vulnerability) error {
	bytes, err := json.Marshal(vulnerability.Semver)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO vulnerabilities (id, name, title, publication_time, semver, severity) VALUES ($1, $2, $3, $4, $5, $6)",
		vulnerability.Id, vulnerability.PackageName, vulnerability.Title, vulnerability.PublicationTime, bytes, vulnerability.Severity)
	return err
}

func DbGetVulnerabilitiesForPackages(packages []string) ([]Vulnerability, error) {
	query, args, err := sqlx.In("SELECT id, name, title, publication_time, semver, severity FROM vulnerabilities WHERE name IN (?) ORDER BY name, publication_time DESC", packages)
	if err != nil {
		return nil, errors.Wrap(err, "could not create query for vulnerabilities for a list of packages")
	}
	query = db.Rebind(query)

	var rows []VulnerabilityRow
	if err := db.Select(&rows, query, args...); err != nil {
		return nil, errors.Wrap(err, "could not get vulnerabilities for a list of packages")
	}
	var vulnerabilities []Vulnerability
	for _, row := range rows {
		v := Vulnerability{Id: row.Id, PackageName: row.Name, Title: row.Title, Severity: Severity(row.Severity)}
		v.PublicationTime, err = time.Parse(time.RFC3339Nano, row.PublicationTime)
		if err != nil {
			log.Println("could not parse time", row.PublicationTime, err)
			continue
		}
		if err := json.Unmarshal(row.Semver, &v.Semver); err != nil {
			log.Println("could not unmarschal semver", row.Semver, err)
			continue
		}
		vulnerabilities = append(vulnerabilities, v)
	}
	return vulnerabilities, nil
}

func connect() {
	source := Config.Database.Source
	var err error
	db, err = sqlx.Connect("sqlite3", source)
	if err != nil {
		log.Panicln("could not open", source, err)
	}
}

func expire() {
	now := time.Now()
	log.Println("run expire")

	result := db.MustExec("DELETE FROM packages WHERE expire_time < $1", now)
	if n, err := result.RowsAffected(); n > 0 && err == nil {
		log.Printf("expired %d packages\n", n)
	}

	result = db.MustExec("DELETE FROM versions WHERE expire_time < $1", now)
	if n, err := result.RowsAffected(); n > 0 && err == nil {
		log.Printf("expired %d versions\n", n)
	}
}

func scheduleExpire() {
	for {
		expire()
		time.Sleep(time.Hour)
	}
}

func runMigrations() {
	Migrate([]Migration{
		{
			Name: "create tables",
			Sql: `
				CREATE TABLE packages (name TEXT, info TEXT, create_time TEXT, expire_time TEXT);
				CREATE UNIQUE INDEX packages_name ON packages (name);

				CREATE TABLE versions (name TEXT, version TEXT, content TEXT, create_time TEXT, expire_time TEXT);
				CREATE UNIQUE INDEX versions_name_version ON versions (name, version);

				CREATE TABLE files (id TEXT, content TEXT, create_time TEXT);
				CREATE UNIQUE INDEX files_id ON files (id);
			`,
		},
		{
			Name: "add latest_version",
			Sql: `
				ALTER TABLE packages ADD COLUMN latest_version TEXT;
			`,
		},
		{
			Name: "create vulnerabilities table",
			Sql: `
				CREATE TABLE vulnerabilities (id TEXT, name TEXT, title TEXT, publication_time TEXT, semver TEXT, severity TEXT);
				CREATE UNIQUE INDEX vulnerabilities_id ON vulnerabilities (id);
				CREATE INDEX vulnerabilities_name ON vulnerabilities (name);
			`,
		},
	})
}

func SetupDb() {
	connect()
	runMigrations()
	go scheduleExpire()
}
