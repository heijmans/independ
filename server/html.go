package server

import (
	"fmt"
	gohtml "html"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

const space = "                                                                                                    "

type Node interface {
	WriteTo(b *strings.Builder, indent int)

	// mainly for multipart mail
	WriteTextTo(b *strings.Builder)
}

func RenderNode(node Node) string {
	var b strings.Builder
	node.WriteTo(&b, 0)
	return b.String()
}

func WriteHtml(node Node, writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "text/html")
	writer.WriteHeader(200)
	_, _ = writer.Write([]byte(RenderNode(node)))
}

func WriteHtmlWithStatus(node Node, status int, writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "text/html")
	writer.WriteHeader(status)
	_, _ = writer.Write([]byte(RenderNode(node)))
}

var multiLine = regexp.MustCompile(`\n{3,}`)

func RenderText(node Node) string {
	var b strings.Builder
	node.WriteTextTo(&b)
	s := b.String()
	return multiLine.ReplaceAllString(s, "\n\n")
}

type elementType int

const (
	Standalone elementType = iota
	Block
	Inline
)

type ElementAttr struct {
	key   string
	value string
}

func Attr(key, value string) ElementAttr {
	return ElementAttr{key, value}
}

type Element struct {
	name     string
	typ      elementType
	block    bool
	attrs    []ElementAttr
	children []Node
}

func newElement(name string) *Element {
	typ, ok := tagToType[name]
	if !ok {
		log.Panicln("unknown tag: " + name)
	}
	return &Element{name: name, typ: typ}
}

func (t *Element) child(children ...Node) *Element {
	t.children = append(t.children, children...)
	return t
}

func (t *Element) Attr(key, value string) *Element {
	t.attrs = append(t.attrs, ElementAttr{key, value})
	return t
}

func (t *Element) Add(params ...interface{}) *Element {
	for _, param := range params {
		if param == nil {
			// ignore
		} else if str, ok := param.(string); ok {
			t.child(TextNode(str))
		} else if i, ok := param.(int); ok {
			t.child(TextNode(strconv.Itoa(i)))
		} else if node, ok := param.(Node); ok {
			t.child(node)
		} else if nodes, ok := param.([]Node); ok {
			t.child(nodes...)
		} else if attr, ok := param.(ElementAttr); ok {
			t.Attr(attr.key, attr.value)
		} else if attrs, ok := param.([]ElementAttr); ok {
			for _, attr := range attrs {
				t.Attr(attr.key, attr.value)
			}
		} else {
			log.Panicln("cannot handle param", param)
		}
	}
	return t
}

func (t *Element) WriteTo(b *strings.Builder, indent int) {
	if t.name == "html" {
		b.WriteString("<!DOCTYPE html>\n")
	}

	b.WriteRune('<')
	b.WriteString(t.name)
	for _, attr := range t.attrs {
		b.WriteRune(' ')
		b.WriteString(attr.key)
		b.WriteRune('=')
		b.WriteRune('"')
		b.WriteString(gohtml.EscapeString(attr.value))
		b.WriteRune('"')
	}

	if t.typ == Standalone {
		b.WriteString(" />")
	} else if t.typ == Block {
		b.WriteRune('>')
		if len(t.children) > 0 {
			for _, child := range t.children {
				if child != nil {
					b.WriteRune('\n')
					b.WriteString(space[:indent+2])
					child.WriteTo(b, indent+2)
				}
			}
			b.WriteRune('\n')
			b.WriteString(space[:indent])
		}
		b.WriteRune('<')
		b.WriteRune('/')
		b.WriteString(t.name)
		b.WriteRune('>')
	} else {
		b.WriteRune('>')
		if len(t.children) > 0 {
			for _, child := range t.children {
				if child != nil {
					child.WriteTo(b, indent)
				}
			}
		}
		b.WriteRune('<')
		b.WriteRune('/')
		b.WriteString(t.name)
		b.WriteRune('>')
	}
}

func (t *Element) WriteTextTo(b *strings.Builder) {
	if t.typ == Block {
		if len(t.children) > 0 {
			for _, child := range t.children {
				if child != nil {
					child.WriteTextTo(b)
					b.WriteRune('\n')
				}
			}
		}
	} else {
		if len(t.children) > 0 {
			for _, child := range t.children {
				if child != nil {
					child.WriteTextTo(b)
				}
			}
		}
	}
}

type TextNode string

func (t TextNode) WriteTo(b *strings.Builder, indent int) {
	b.WriteString(gohtml.EscapeString(string(t)))
}

func (t TextNode) WriteTextTo(b *strings.Builder) {
	b.WriteString(string(t))
}

type UnsafeRawContent string

func (t UnsafeRawContent) WriteTo(b *strings.Builder, indent int) {
	b.WriteString(string(t))
}

func (t UnsafeRawContent) WriteTextTo(b *strings.Builder) {
	// not supported in plain text, just ignore
}

var tagToType = map[string]elementType{
	"div":      Block,
	"span":     Inline,
	"html":     Block,
	"head":     Block,
	"body":     Block,
	"meta":     Standalone,
	"title":    Inline,
	"link":     Standalone,
	"script":   Block,
	"h1":       Block,
	"h2":       Block,
	"h3":       Block,
	"h4":       Block,
	"p":        Block,
	"a":        Inline,
	"br":       Standalone,
	"hr":       Standalone,
	"ul":       Block,
	"li":       Block,
	"b":        Inline,
	"pre":      Inline,
	"img":      Standalone,
	"table":    Block,
	"thead":    Block,
	"tbody":    Block,
	"tr":       Block,
	"th":       Block,
	"td":       Block,
	"form":     Block,
	"input":    Standalone,
	"button":   Inline,
	"textarea": Inline,
}

type specParser struct {
	h      string
	i      int
	n      int
	params []interface{}
}

func (sp *specParser) more() bool {
	return sp.i < sp.n
}

func (sp *specParser) cur() uint8 {
	if sp.i >= sp.n {
		return 0
	}
	return sp.h[sp.i]
}

func (sp *specParser) next() {
	sp.i++
}

func (sp *specParser) panicExpected(s string) {
	log.Panicf("expected %s in \"%v\" @ %v (...%v)", s, sp.h, sp.i, sp.h[sp.i:])
}

func (sp *specParser) skip(ch uint8) {
	if !sp.more() || sp.cur() != ch {
		sp.panicExpected(fmt.Sprintf("'%c'", ch))
	}
	sp.i++
}

func isLetter(ch uint8) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_'
}

func (sp *specParser) parseName() string {
	start := sp.i
	if !isLetter(sp.cur()) {
		sp.panicExpected("[A-Za-z0-9-_]")
	}
	for sp.more() && isLetter(sp.cur()) {
		sp.next()
	}
	return sp.h[start:sp.i]
}

func (sp *specParser) parseSpec() *Element {
	first := sp.cur()
	var tag string
	if first == '.' || first == '#' {
		tag = "div"
	} else {
		tag = sp.parseName()
	}
	el := newElement(tag)

	var classes []string
	for sp.more() && sp.cur() != ' ' {
		first = sp.cur()
		if first == '.' {
			sp.next()
			classes = append(classes, sp.parseName())
		} else if first == '#' {
			sp.next()
			id := sp.parseName()
			el.Attr("id", id)
		} else {
			sp.panicExpected("' ', '.' or '#'")
		}
	}
	if len(classes) > 0 {
		el.Attr("class", strings.Join(classes, " "))
	}

	return el
}

func (sp *specParser) parseAttr() ElementAttr {
	key := sp.parseName()
	sp.skip('=')
	var value string
	if sp.cur() == '\'' {
		sp.next()
		start := sp.i
		for sp.cur() != '\'' {
			sp.next()
		}
		value = sp.h[start:sp.i]
		sp.skip('\'')
	} else if sp.cur() == '%' {
		sp.next()
		spec := sp.cur()
		if spec == 's' {
			sp.next()
			value = sp.params[0].(string)
			sp.params = sp.params[1:]
		} else if spec == 'd' {
			sp.next()
			value = strconv.Itoa(sp.params[0].(int))
			sp.params = sp.params[1:]
		} else {
			sp.panicExpected("%s or %d")
		}
	} else {
		start := sp.i
		for sp.more() && sp.cur() != ' ' {
			sp.next()
		}
		value = sp.h[start:sp.i]
	}
	return ElementAttr{key, value}
}

func H(h string, p ...interface{}) *Element {
	var top, cur *Element
	n := len(h)
	sp := specParser{h, 0, n, p}
	if n == 0 {
		top = newElement("div")
		cur = top
	} else {
		for sp.more() {
			el := sp.parseSpec()
			if cur == nil {
				top = el
			} else {
				cur.child(el)
			}
			cur = el
			if sp.more() {
				sp.skip(' ')
			}
			for sp.more() && sp.cur() != '>' {
				attr := sp.parseAttr()
				cur.Attr(attr.key, attr.value)
				if sp.more() {
					sp.skip(' ')
				}
			}
			if sp.more() {
				sp.skip('>')
				sp.skip(' ')
			}
		}
	}

	cur.Add(sp.params...)

	return top
}
