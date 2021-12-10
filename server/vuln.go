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
	PublicationTime string     `json:"publicationTime"`
	Semver          SemverSpec `json:"semver"`
	Severity        Severity   `json:"severity"`
}

type VulnerabilityResponse struct {
	Status          string          `json:"status"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
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

func init() {
	go func() {
		time.Sleep(100 * time.Millisecond)
		last, err := DbLastVulnerability()
		if err != nil {
			log.Panicln("could not get last vuln", err)
		}

		page := 1
	VulnLoop:
		for {
			vulnerabilities, err := GetVulnerabilities(page)
			if err != nil {
				log.Println("could not get vuln, break", err)
				break
			}
			if len(vulnerabilities) == 0 {
				log.Println("received all vulns")
				break
			}
			for _, vulnerability := range vulnerabilities {
				if last != nil && vulnerability.Id == last.Id {
					log.Println("received known vuln: " + last.Id)
					break VulnLoop
				}
				err := DbPutVulnerability(vulnerability)
				if err != nil {
					log.Println("could not put vuln, break", err)
				}
			}
			page++
		}
	}()
}
