package server

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/pkg/errors"
)

type SemverSpec struct {
	Vulnerable []string `json:"vulnerable"`
}

type Severity string

const (
	Low      Severity = "low"
	Medium   Severity = "medium"
	High     Severity = "high"
	Critical Severity = "critical"
)

type Vulnerability struct {
	Id              string     `json:"id"`
	PackageManager  string     `json:"packageManager"`
	PackageName     string     `json:"packageName"`
	Title           string     `json:"title"`
	PublicationTime time.Time  `json:"publicationTime"`
	Semver          SemverSpec `json:"semver"`
	Severity        Severity   `json:"severity"`
}

type VulnerabilityResponse struct {
	Status          string          `json:"status"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
}

type VulnerabilityStats struct {
	LowCount      int `json:"lowCount"`
	MediumCount   int `json:"mediumCount"`
	HighCount     int `json:"highCount"`
	CriticalCount int `json:"criticalCount"`
}

func GetVulnerabilities(page int) ([]Vulnerability, error) {
	url := fmt.Sprintf("https://security.snyk.io/api/listing?type=npm&pageNumber=%d", page)
	var response VulnerabilityResponse
	body, err := getBody(url)
	if err != nil {
		return nil, errors.Wrap(err, "could not get vulnerabilities")
	}
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, errors.Wrap(err, "could not parse json for vulnerabilities")
	}
	if response.Status != "ok" {
		return nil, errors.Wrap(err, "response status for vulnerabilities: "+response.Status)
	}
	return response.Vulnerabilities, nil
}

func GetVulnerabilityStats(vulnerabilities []Vulnerability) VulnerabilityStats {
	var stats VulnerabilityStats
	for _, vulnerability := range vulnerabilities {
		severity := vulnerability.Severity
		if severity == Low {
			stats.LowCount++
		} else if severity == Medium {
			stats.MediumCount++
		} else if severity == High {
			stats.HighCount++
		} else if severity == Critical {
			stats.CriticalCount++
		}
	}
	return stats
}

func UpdateVulnerabilities() {
	last, err := DbLastVulnerability()
	if err != nil {
		log.Println("could not get last vuln", err)
		return
	}

	page := 1
	for {
		vulnerabilities, err := GetVulnerabilities(page)
		if err != nil {
			log.Println("could not get vuln, break", err)
			return
		}
		if len(vulnerabilities) == 0 {
			log.Println("received all vulns")
			return
		}
		for _, vulnerability := range vulnerabilities {
			if last != nil && vulnerability.Id == last.Id {
				log.Println("received known vuln: " + last.Id)
				return
			}
			if err := DbPutVulnerability(vulnerability); err != nil {
				log.Println("could not put vuln", err)
			}
		}
		page++
	}
}

func init() {
	go func() {
		time.Sleep(time.Second)
		for {
			UpdateVulnerabilities()
			time.Sleep(4 * time.Hour)
		}
	}()
}
