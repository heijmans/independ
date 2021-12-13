package server

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
)

func getBody(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err // wrap?
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, errors.New(resp.Status + " in " + url)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err // wrap?
	}
	return body, nil
}

type Dist struct {
	FileCount    int   `json:"fileCount"`
	UnpackedSize int64 `json:"unpackedSize"`
}

type DistTags struct {
	Latest string `json:"latest"`
}

type NpmUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type VersionInfo struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Description     string            `json:"description"`
	Homepage        interface{}       `json:"homepage"`
	License         interface{}       `json:"license"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	NpmUser         NpmUser           `json:"_npmUser"`
	Dist            Dist              `json:"dist"`
	Os              []string          `json:"os"`
	Cpu             []string          `json:"cpu"`
}

func (v VersionInfo) GetPublisher() string {
	var res string
	if v.NpmUser.Name != "" {
		res = v.NpmUser.Name
		if v.NpmUser.Email != "" {
			res += " ( " + v.NpmUser.Email + " )"
		}
	} else if v.NpmUser.Email != "" {
		res = v.NpmUser.Email
	}
	return res
}

type PackageInfo struct {
	Name     string                 `json:"name"`
	DistTags DistTags               `json:"dist-tags"`
	Versions map[string]VersionInfo `json:"versions"`
	Time     map[string]time.Time   `json:"time"`
}

func GetPackageInfoRegistry(name string) (*PackageInfo, error) {
	log.Println("get", name, "from registry")
	var packageInfo PackageInfo
	body, err := getBody("https://registry.npmjs.org/" + name)
	if err != nil {
		return nil, errors.Wrap(err, "could not get package "+name)
	}
	if err = json.Unmarshal(body, &packageInfo); err != nil {
		return nil, errors.Wrap(err, "could not parse json for package "+name)
	}
	return &packageInfo, nil
}

func (p *PackageInfo) MaxVersion(constraintRaw string) (VersionInfo, error) {
	var maxVersion *semver.Version
	var maxVersionInfo VersionInfo
	constraint, err := semver.NewConstraint(constraintRaw)
	if err != nil {
		return maxVersionInfo, err
	}
	for versionRaw, info := range p.Versions {
		version, err := semver.NewVersion(versionRaw)
		if err != nil {
			continue
		}
		if ok, _ := constraint.Validate(version); ok {
			if maxVersion == nil || version.GreaterThan(maxVersion) {
				maxVersion = version
				maxVersionInfo = info
			}
		}
	}
	if maxVersion == nil {
		return maxVersionInfo, errors.New("no matching version found in " + p.Name + " constraint " + constraintRaw)
	} else {
		return maxVersionInfo, nil
	}
}

func (p *PackageInfo) LatestVersion() VersionInfo {
	return p.Versions[p.DistTags.Latest]
}

func (p *PackageInfo) LatestTime() time.Time {
	return p.Time[p.DistTags.Latest]
}

type Stats struct {
	Packages           int                `json:"packages"`
	Versions           int                `json:"versions"`
	Files              int                `json:"files"`
	DiskSpace          int64              `json:"diskSpace"`
	VulnerabilityStats VulnerabilityStats `json:"vulnerabilityStats"`
}

type Version struct {
	Info            VersionInfo         `json:"info"`
	Time            time.Time           `json:"time"`
	Dependencies    map[string][]string `json:"dependencies"`
	Publishers      map[string]int      `json:"publishers"`
	Vulnerabilities []Vulnerability     `json:"vulnerabilities"`
	Stats           Stats               `json:"stats"`
	Errors          []string            `json:"error"`
}

func NewVersion(versionInfo VersionInfo, time time.Time) *Version {
	stats := Stats{
		Packages:  1,
		Versions:  1,
		Files:     versionInfo.Dist.FileCount,
		DiskSpace: versionInfo.Dist.UnpackedSize,
	}
	publishers := map[string]int{}
	publisher := versionInfo.GetPublisher()
	if publisher != "" {
		publishers[publisher] = 1
	}
	return &Version{
		Info:         versionInfo,
		Time:         time,
		Dependencies: map[string][]string{},
		Publishers:   publishers,
		Stats:        stats,
	}
}

func HasMatchingVersion(versions []string, constraint *semver.Constraints) bool {
	ok := false
	for _, vRaw := range versions {
		v, err := semver.NewVersion(vRaw)
		if err != nil {
			continue
		}
		valid, _ := constraint.Validate(v)
		if valid {
			ok = true
			break
		}
	}
	return ok
}

func (v *Version) GatherVulnerabilities() error {
	packageNames := []string{v.Info.Name}
	for name := range v.Dependencies {
		packageNames = append(packageNames, name)
	}
	allVulnerabilities, err := DbGetVulnerabilitiesForPackages(packageNames)
	if err != nil {
		return errors.Wrapf(err, "could not get vulnerabilities for package %s", v.Info.Name)
	}
	var vulnerabilities []Vulnerability
	for _, vulnerability := range allVulnerabilities {
		match := false
		name := vulnerability.PackageName
		var depVersions []string
		if name == v.Info.Name {
			depVersions = []string{v.Info.Version}
		} else {
			depVersions = v.Dependencies[name]
		}
		for _, depVersion := range depVersions {
			depV, err := semver.NewVersion(depVersion)
			if err != nil {
				log.Println("err in version", depVersion, err)
				continue
			}
			for _, expr := range vulnerability.Semver.Vulnerable {
				c, err := semver.NewConstraint(expr)
				if err != nil {
					log.Println("err in constraint", expr, err)
					continue
				}
				if c.Check(depV) {
					match = true
				}
			}
		}
		if match {
			vulnerabilities = append(vulnerabilities, vulnerability)
		}
	}
	v.Vulnerabilities = vulnerabilities
	v.Stats.VulnerabilityStats = GetVulnerabilityStats(vulnerabilities)

	return nil
}

func (p VersionInfo) GatherDependencies(parent *Version, alsoDev bool) {
	if len(p.Dependencies) > 0 || (alsoDev && len(p.DevDependencies) > 0) {
		var names []string
		var constraints []string
		var futures []*Future
		for name, constraintRaw := range p.Dependencies {
			names = append(names, name)
			constraints = append(constraints, constraintRaw)
			futures = append(futures, packagePool.ProcessKey(name))
		}
		if alsoDev {
			for name, constraintRaw := range p.DevDependencies {
				names = append(names, name)
				constraints = append(constraints, constraintRaw)
				futures = append(futures, packagePool.ProcessKey(name))
			}
		}
		for i, future := range futures {
			name := names[i]
			constraintRaw := constraints[i]
			result := future.Await()
			if result.Error != nil {
				parent.Errors = append(parent.Errors, "could not get "+name+": "+result.Error.Error())
				continue
			}
			packageInfo := result.Data.(*PackageInfo)
			constraint, err := semver.NewConstraint(constraintRaw)
			if err != nil {
				parent.Errors = append(parent.Errors, "invalid constraint for "+name+" constraint "+constraintRaw+": "+err.Error())
				continue
			}
			childVersion, err := packageInfo.MaxVersion(constraintRaw)
			if err != nil {
				parent.Errors = append(parent.Errors, "no matching version for "+name+" constraint "+constraintRaw+": "+err.Error())
				continue
			}
			if !childVersion.MatchPlatform("linux", "x64") {
				continue
			}
			gather := false
			dependencies := parent.Dependencies
			stats := &parent.Stats
			if versions, hasDepend := dependencies[name]; hasDepend {
				if !HasMatchingVersion(versions, constraint) {
					dependencies[name] = append(dependencies[name], childVersion.Version)
					gather = true
				}
			} else {
				dependencies[name] = []string{childVersion.Version}
				gather = true
				stats.Packages++
			}
			if gather {
				publisher := childVersion.GetPublisher()
				parent.Publishers[publisher]++
				stats.Versions++
				stats.Files += childVersion.Dist.FileCount
				stats.DiskSpace += childVersion.Dist.UnpackedSize
				childVersion.GatherDependencies(parent, false)
			}
		}
	}
}

func strArrContain(array []string, s string) bool {
	for _, item := range array {
		if item == s {
			return true
		}
	}
	return false
}

func (p VersionInfo) MatchPlatform(os string, cpu string) bool {
	if len(p.Os) > 0 {
		if !strArrContain(p.Os, os) {
			return false
		}
	}
	if len(p.Cpu) > 0 {
		if !strArrContain(p.Cpu, cpu) {
			return false
		}
	}
	return true
}

func (p *PackageInfo) GatherDependencies(versionRaw string) (*Version, error) {
	var versionInfo VersionInfo
	if versionRaw != "" {
		var ok bool
		versionInfo, ok = p.Versions[versionRaw]
		if !ok {
			return nil, errors.Errorf("could not find version %s in %s", versionRaw, p.Name)
		}
	} else {
		versionInfo = p.LatestVersion()
	}
	parent := NewVersion(versionInfo, p.Time[versionInfo.Version])
	versionInfo.GatherDependencies(parent, false)
	if err := parent.GatherVulnerabilities(); err != nil {
		return nil, errors.Wrapf(err, "could not gather vulns for %s version %s", p.Name, versionRaw)
	}
	return parent, nil
}

func calcExpire(lastUpdate time.Time) time.Time {
	now := time.Now()
	age := now.Sub(lastUpdate)
	expire := age / 100
	if expire.Hours() < 1 {
		expire = time.Hour
	} else if expire.Hours() > 24 {
		expire = 24 * time.Hour
	}
	return now.Add(expire)
}

type PackageInfoPerformer struct{}

func (p PackageInfoPerformer) Get(name string) Data {
	packageInfo, err := DbGetPackage(name)
	if err != nil {
		return nil
	}
	return packageInfo
}

func (p PackageInfoPerformer) Put(name string, data Data) {
	packageInfo := data.(*PackageInfo)
	err := DbPutPackage(name, packageInfo, calcExpire(packageInfo.LatestTime()))
	if err != nil {
		log.Println("could not put package "+name+" in db", err)
	}
}

func (p PackageInfoPerformer) Perform(name string) Result {
	packageInfo, err := GetPackageInfoRegistry(name)
	if err != nil {
		return Result{Error: err}
	}
	return Result{Data: packageInfo}
}

var packagePool *SmartWorkPool

func GetPackageInfo(name string) (*PackageInfo, error) {
	result := packagePool.ProcessKey(name).Await()
	if result.Error != nil {
		return nil, result.Error
	}
	return result.Data.(*PackageInfo), nil
}

type VersionPerformer struct{}

func parseVersionKey(key string) (string, string) {
	parts := strings.Split(key, "\t")
	name := parts[0]
	versionRaw := parts[1]
	return name, versionRaw
}

func (p VersionPerformer) Get(key string) Data {
	name, versionRaw := parseVersionKey(key)
	version, err := DbGetVersion(name, versionRaw)
	if err != nil {
		return nil
	}
	return version
}

func (p VersionPerformer) Put(key string, data Data) {
	name, versionRaw := parseVersionKey(key)
	version := data.(*Version)
	err := DbPutVersion(name, versionRaw, version, calcExpire(version.Time))
	if err != nil {
		log.Println("could not put version "+key+" in db", err)
	}
}

func (p VersionPerformer) Perform(key string) Result {
	name, versionRaw := parseVersionKey(key)
	packageInfo, err := GetPackageInfo(name)
	if err != nil {
		return Result{Error: err}
	}
	version, err := packageInfo.GatherDependencies(versionRaw)
	if err != nil {
		return Result{Error: err}
	}
	return Result{Data: version}
}

var versionPool *SmartWorkPool

func GetVersion(name string, version string) (*Version, error) {
	result := versionPool.ProcessKey(name + "\t" + version).AwaitTimeout(time.Second * 1)
	if result.Error != nil {
		return nil, result.Error
	}
	return result.Data.(*Version), nil
}

type FilePerformer struct{}

func fileIsReady(version *Version) bool {
	return len(version.Dependencies) > 0 || len(version.Info.Dependencies) == 0
}

func (p FilePerformer) Get(id string) Data {
	version, err := DbGetFile(id)
	if err != nil || !fileIsReady(version) {
		return nil
	}
	return version
}

func (p FilePerformer) Put(id string, data Data) {
	version := data.(*Version)
	err := DbPutFile(id, version)
	if err != nil {
		log.Println("could not put file "+id+" in db", err)
	}
}

func (p FilePerformer) Perform(id string) Result {
	version, err := DbGetFile(id)
	if err != nil {
		return Result{Error: err}
	}
	version.Info.GatherDependencies(version, true)
	return Result{Data: version}
}

var filePool *SmartWorkPool

func GetFile(id string) (*Version, error) {
	result := filePool.ProcessKey(id).AwaitTimeout(time.Second * 1)
	if result.Error != nil {
		return nil, result.Error
	}
	return result.Data.(*Version), nil
}

func init() {
	packagePool = NewSmartWorkPool(PackageInfoPerformer{})
	packagePool.Start(8)

	versionPool = NewSmartWorkPool(VersionPerformer{})
	versionPool.Start(4)

	filePool = NewSmartWorkPool(FilePerformer{})
	filePool.Start(4)
}
