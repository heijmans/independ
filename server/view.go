package server

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

func npmHref(name string, version string) string {
	if version == "" {
		return "/npm/" + name
	} else {
		return "/npm/" + name + "/" + version
	}
}

var startTime = time.Now()

func publicHref(path string) string {
	filePath := "public" + path

	// if we cannot stat the file, it is perhaps embedded, so use the launch time
	modTime := startTime
	if stat, err := os.Stat(filePath); err == nil {
		modTime = stat.ModTime()
	}
	return fmt.Sprintf("%s?t=%d", path, modTime.UnixMilli())
}

func Layout(title string, content Node) Node {
	var buttons []Node
	for _, title := range Config.Pages.Buttons {
		path := "/pages/" + strings.ReplaceAll(strings.ToLower(title), " ", "-")
		buttons = append(buttons, H("a href=%s", path, title))
	}

	return H("html",
		H("head",
			H("meta charset=UTF-8"),
			H("meta name=viewport content=%s", "width=640"),
			H("title", title+" | independ"),
			H("link rel=stylesheet href=%s", publicHref("/main.css")),
		),
		H("body",
			H(".header",
				H("a href=/", "independ"),
				buttons,
			),
			content,
			H("script src=%s", publicHref("/main.js")),
		),
	)
}

func renderVersions(name string, versions []string) Node {
	var links []Node
	for _, v := range versions {
		links = append(links, TextNode(", "), H("a href=%s", npmHref(name, v), v))
	}
	return H("td", links[1:])
}

func sortedDependencyNames(dependencies map[string][]string) []string {
	var names []string
	for name := range dependencies {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type IntEntry struct {
	Key   string
	Value int
}

type IntEntries []IntEntry

func (l IntEntries) Len() int {
	return len(l)
}

func (l IntEntries) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

// Less sorts descending
func (l IntEntries) Less(i, j int) bool {
	left := l[i]
	right := l[j]
	if left.Value == right.Value {
		return left.Key < right.Key
	} else {
		return left.Value > right.Value
	}
}

func sortedMapByIntValue(m map[string]int) IntEntries {
	var list IntEntries
	for key, value := range m {
		list = append(list, IntEntry{key, value})
	}
	sort.Sort(list)
	return list
}

type Tab struct {
	Title   string
	Id      string
	Content Node
}

func RenderTabs(tabs []Tab) Node {
	/*
		<div class="tab-buttons">
			<div class="tab-button tab-button-active" data-tab-id="depends">Dependencies</div>
			<div class="tab-button" data-tab-id="publishers">Publishers</div>
		</div>
		<div class="tabs">
			<div id="depends" class="tab tab-active">
				...
			</div>
			<div id="publishers" class="tab">
				...
			</div>
		</div>
	*/

	var tabButtons []Node
	var tabContents []Node

	for i, tab := range tabs {
		buttonSpec := ".tab-button"
		contentSpec := ".tab"
		if i == 0 {
			buttonSpec += ".tab-button-active"
			contentSpec += ".tab-active"
		}
		tabButtons = append(tabButtons, H(buttonSpec, Attr("data-tab-id", tab.Id), tab.Title))
		tabContents = append(tabContents, H(contentSpec, Attr("id", tab.Id), tab.Content))
	}

	return H("div",
		H(".tab-buttons", tabButtons),
		H(".tabs", tabContents),
	)
}

func VersionView(version *Version) Node {
	info := version.Info
	var description, homepage, license, npmUser Node
	if info.Description != "" {
		description = H("tr", H("th", "description:"), H("td", info.Description))
	}
	if info.Homepage != nil && info.Homepage != "" {
		var node Node
		if s, ok := info.Homepage.(string); ok {
			node = H("a href=%s target=_blank", s, s)
		} else {
			node = TextNode(fmt.Sprint(info.Homepage))
		}
		homepage = H("tr", H("th", "homepage:"), H("td", node))
	}
	if info.License != nil && info.License != "" {
		license = H("tr", H("th", "license:"), H("td", fmt.Sprint(info.License)))
	}
	publisher := info.GetPublisher()
	if publisher != "" {
		npmUser = H("tr", H("th", "published by:"), H("td", publisher))
	}
	publishedAt := H("tr", H("th", "published at:"), H("td", version.Time.Format("2006-01-02 15:04 Z07:00")))

	var errors Node
	if len(version.Errors) > 0 {
		var list []Node
		for _, e := range version.Errors {
			list = append(list, H("li", e))
		}
		errors = H(".errors",
			H("h3", "Errors"),
			H("ul", list),
		)
	}

	stats := H("div",
		H("h3", fmt.Sprintf("packages: %d \u00a0 versions: %d \u00a0 publishers: %d", version.Stats.Packages, version.Stats.Versions, len(version.Publishers))),
		H("h3", fmt.Sprintf("files: %d \u00a0 disk space: %.2f MB", version.Stats.Files, float64(version.Stats.DiskSpace)/1e6)),
	)

	var depTable Node
	if len(version.Dependencies) > 0 {
		var dependencies []Node
		for _, name := range sortedDependencyNames(version.Dependencies) {
			versions := version.Dependencies[name]
			dependencies = append(dependencies, H("tr",
				H("td", H("a href=%s", npmHref(name, ""), name)),
				renderVersions(name, versions),
			))
		}
		depTable = H("table", H("tr", H("th", "name"), H("th", "versions")), dependencies)
	}

	var pubTable Node
	if len(version.Publishers) > 0 {
		var publishers []Node
		for _, entry := range sortedMapByIntValue(version.Publishers) {
			publishers = append(publishers, H("tr", H("td", entry.Key), H("td", entry.Value)))
		}
		pubTable = H("table", H("tr", H("th", "publisher"), H("th", "count")), publishers)
	}

	tabs := []Tab{
		Tab{"Dependencies", "dependencies", depTable},
		Tab{"Publishers", "publishers", pubTable},
	}

	title := info.Name + " " + info.Version + " dependencies"
	return Layout(title,
		H(".main",
			H("h1", title),
			H("table",
				description,
				homepage,
				license,
				npmUser,
				publishedAt,
			),
			errors,
			stats,
			H("hr"),
			RenderTabs(tabs),
		),
	)
}

func WaitView(name string) Node {
	title := "Waiting for " + name + "..."
	message := "Please wait while the dependencies of " + name + " are being fetched. " +
		"This may take a minute or so, depending on the number of dependencies. " +
		"This page will automatically refresh when it is ready."
	script := UnsafeRawContent("setTimeout(() => document.location.reload(), 2000);")

	return Layout(title,
		H(".main",
			H("h1", title),
			H("p", message),
			H("script", script),
		),
	)
}

func linkPackage(name string) Node {
	return H("a href=%s", "/npm/"+name, name)
}

func HomeView() Node {
	title := "independ: know your dependencies"
	return Layout(title,
		H(".main",
			H("h1", title),
			H("h3", "Check out some examples:"),
			H("p",
				linkPackage("@angular/cli"),
				H("br"),
				linkPackage("esbuild"),
				H("br"),
				linkPackage("typescript"),
				H("br"),
				linkPackage("react"),
				H("br"),
				linkPackage("webpack"),
			),
			H("h3", "Go to another package:"),
			H("form action=/go > p",
				H("input name=package placeholder=%s required=required", "Package name"),
				H("button", "Go"),
			),
			H("h3", "Upload package.json:"),
			H("form method=POST action=/upload enctype=multipart/form-data > p",
				H("input type=file name=file required=required"),
				H("button", "Upload"),
			),
		),
	)
}

func ErrorView(title string, err string, trace string) Node {
	return Layout(title,
		H("div",
			H("h3", title),
			H("p", err),
			H("h4", "Technical Information"),
			H("pre", trace),
		),
	)
}

func PageView(page Page) Node {
	content := UnsafeRawContent(page.Content)
	return Layout(page.Title, content)
}
