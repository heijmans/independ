package server

import (
	"io/ioutil"
	"regexp"

	"github.com/gomarkdown/markdown"
	"github.com/pkg/errors"
)

type Page struct {
	Title   string
	Content string
}

var H1RE = regexp.MustCompile(`^\s*# (.*)\n`)

func GetPage(path string) (Page, error) {
	page := Page{Title: path}
	filePath := Config.Pages.Path + "/" + path + ".md"
	md, err := ioutil.ReadFile(filePath)
	if err != nil {
		return page, errors.Wrap(err, "could not read page: "+filePath)
	}
	page.Content = string(markdown.ToHTML(md, nil, nil))

	matches := H1RE.FindStringSubmatch(string(md))
	if len(matches) == 2 {
		page.Title = matches[1]
	}

	return page, nil
}
