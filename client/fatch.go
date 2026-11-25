package client

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	stdhtml "html"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"github.com/PuerkitoBio/goquery"
	"github.com/cys001x/cf-tool-x/util"
)

// imageTimeout bounds a single image download. Image hosts used by old blog
// entries are often dead and would otherwise block the whole command.
const imageTimeout = 20 * time.Second

func nodeText(sel *goquery.Selection, visited map[*html.Node]bool) string {
	var sb strings.Builder
	sel.Contents().Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		// For div elements, skip nodes already visited to avoid parsing them twice
		if goquery.NodeName(s) == "div" {
			if visited[node] {
				return
			}
			visited[node] = true
		}
		switch goquery.NodeName(s) {
		case "br":
			sb.WriteByte('\n')
			sb.WriteByte('\n')
		case "#text":
			sb.WriteString(s.Text())
		case "p", "div":
			sb.WriteString(nodeText(s, visited))
			sb.WriteByte('\n')
			sb.WriteByte('\n')
		case "ul", "ol":
			// When a nested list sits inside <li>, add a line break so its items start on a new line
			if goquery.NodeName(sel) == "li" {
				sb.WriteByte('\n')
				sb.WriteByte('\n')
			}
			s.ChildrenFiltered("li").Each(func(_ int, li *goquery.Selection) {
				sb.WriteString("- ")
				sb.WriteString(strings.TrimSpace(nodeText(li, visited)))
				sb.WriteByte('\n')
				sb.WriteByte('\n')
			})
		default:
			sb.WriteString(nodeText(s, visited))
		}
	})
	return sb.String()
}

func sectionText(sel *goquery.Selection, visited map[*html.Node]bool) string {
	var sb strings.Builder
	// title
	title := strings.TrimSpace(sel.Find(".section-title").Text())
	if title != "" {
		sb.WriteString("## ")
		sb.WriteString(title)
		sb.WriteString("\n\n")
	}
	sel.Find(".section-title").Remove()
	text := strings.TrimSpace(nodeText(sel, visited))
	// collapse multiple blank lines
	reg := regexp.MustCompile(`\n{3,}`)
	text = reg.ReplaceAllString(text, "\n\n")
	sb.WriteString(text)
	sb.WriteString("\n\n")
	return sb.String()
}

// renderSpoiler renders a .spoiler div: expands the content, fetching tutorial
// text for Editorial spoilers.
func (c *Client) renderSpoiler(s *goquery.Selection, csrf, blogURL string, visited map[*html.Node]bool) string {
	titleText := strings.TrimSpace(s.Find("b.spoiler-title").Text())
	content := s.Find(".spoiler-content")

	var sb strings.Builder
	sb.WriteString("### ")
	sb.WriteString(titleText)
	sb.WriteString("\n\n")

	// Check for lazy-loaded problem tutorial
	tutDiv := content.Find(".problemTutorial")
	if tutDiv.Length() > 0 {
		problemCode, _ := tutDiv.Attr("problemcode")
		if problemCode != "" && csrf != "" {
			html := c.fetchProblemTutorial(problemCode, csrf, blogURL)
			if html != "" {
				doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
				if err == nil {
					sb.WriteString(nodeText(doc.Find(".ttypography").First(), visited))
					sb.WriteString("\n")
					return sb.String()
				}
			}
		}
		sb.WriteString("*(Tutorial not available)*\n")
		return sb.String()
	}

	// Code block inside spoiler
	pre := content.Find("pre code")
	if pre.Length() > 0 {
		sb.WriteString("```\n")
		sb.WriteString(stdhtml.UnescapeString(pre.Text()))
		sb.WriteString("\n```\n")
		return sb.String()
	}

	// Generic spoiler content
	sb.WriteString(nodeText(content, visited))
	sb.WriteString("\n")
	return sb.String()
}

// extractCSRF extracts the X-Csrf-Token from a page body.
func extractCSRF(body []byte) string {
	reg := regexp.MustCompile(`X-Csrf-Token" content="([^"]+)"`)
	if m := reg.FindSubmatch(body); m != nil {
		return string(m[1])
	}
	return ""
}

// fetchProblemTutorial fetches the editorial text for a single problem via the
// Codeforces AJAX endpoint, returning rendered HTML.
func (c *Client) fetchProblemTutorial(problemCode, csrf, referer string) string {
	form := url.Values{"problemCode": {problemCode}}
	req, err := http.NewRequest("POST", "https://codeforces.com/data/problemTutorial",
		strings.NewReader(form.Encode()))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Csrf-Token", csrf)
	req.Header.Set("Referer", referer)

	resp, err := c.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	var result struct {
		Success string `json:"success"`
		HTML    string `json:"html"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Success != "true" {
		return ""
	}
	return result.HTML
}

// FetchUrlContentToMdFile fetches the Codeforces page at URL and renders it
// as a markdown file.
func (c *Client) FetchUrlContentToMdFile(URL, path string, fileName string, mu *sync.Mutex) error {
	body, err := util.GetBody(c.client, URL)
	if err != nil {
		return err
	}
	return c.writeContentToMdFile(body, URL, path, fileName)
}

// resolveImageURL turns src into an absolute URL relative to pageURL.
func resolveImageURL(pageURL, src string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	if strings.HasPrefix(src, "//") {
		return "https:" + src
	}
	u, err := url.Parse(src)
	if err != nil {
		return ""
	}
	if u.IsAbs() {
		return u.String()
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	return base.ResolveReference(u).String()
}

func imageExtFromContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/svg+xml":
		return ".svg"
	case "image/webp":
		return ".webp"
	}
	return ""
}

// imageFileName builds a stable, collision-free file name from an image URL.
func imageFileName(u *url.URL, src, contentType string) string {
	base := path.Base(u.Path)
	stem := strings.TrimSuffix(base, path.Ext(base))
	stem = regexp.MustCompile(`[^\w.-]+`).ReplaceAllString(stem, "_")
	stem = strings.Trim(stem, "._")
	if stem == "" {
		stem = "image"
	}
	ext := strings.ToLower(path.Ext(base))
	if ext == "" {
		ext = imageExtFromContentType(contentType)
	}
	if ext == "" {
		ext = ".img"
	}
	h := fnv.New32a()
	h.Write([]byte(src))
	return fmt.Sprintf("%s_%08x%s", stem, h.Sum32(), ext)
}

// downloadImage fetches one image into dir. It returns the path relative to
// the markdown file, or "" when the image cannot be saved.
func (c *Client) downloadImage(pageURL, src, dir string) string {
	resolved := resolveImageURL(pageURL, src)
	if resolved == "" {
		return ""
	}
	u, err := url.Parse(resolved)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	if err = os.MkdirAll(dir, os.ModePerm); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), imageTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", resolved, nil)
	if err != nil {
		return ""
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil || len(data) == 0 {
		return ""
	}
	name := imageFileName(u, resolved, resp.Header.Get("Content-Type"))
	if err = ioutil.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
		return ""
	}
	return "assets/" + name
}

// localizeImages downloads every statement image into "<dir>/assets" and
// rewrites the <img> node as markdown so the statement works offline. Images
// that cannot be fetched keep their absolute URL.
func (c *Client) localizeImages(doc *goquery.Document, pageURL, dir string) int {
	assetsDir := filepath.Join(dir, "assets")
	saved := map[string]string{}
	count := 0
	doc.Find(".problem-statement img, .ttypography img").Each(func(_ int, s *goquery.Selection) {
		src, _ := s.Attr("src")
		if src == "" {
			src, _ = s.Attr("data-src")
		}
		if src == "" || strings.HasPrefix(strings.ToLower(src), "data:") {
			return
		}
		ref, ok := saved[src]
		if !ok {
			ref = c.downloadImage(pageURL, src, assetsDir)
			if ref == "" {
				ref = resolveImageURL(pageURL, src)
			}
			saved[src] = ref
		}
		alt := regexp.MustCompile(`[\[\]\r\n]+`).ReplaceAllString(s.AttrOr("alt", ""), " ")
		node := s.Get(0)
		text := &html.Node{Type: html.TextNode, Data: fmt.Sprintf("![%v](%v)", strings.TrimSpace(alt), ref)}
		node.Parent.InsertBefore(text, node)
		node.Parent.RemoveChild(node)
		count++
	})
	return count
}

// writeContentToMdFile renders an already fetched problem page as markdown.
// Splitting the fetch from the rendering lets Sync reuse a single request for
// both the samples and the statement.
func (c *Client) writeContentToMdFile(body []byte, URL, path string, fileName string) error {
	csrf := extractCSRF(body)

	if _, err := findHandle(body); err != nil {
		return err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("cannot parse HTML: %v", err)
	}

	// Pull statement images down so the markdown works offline.
	c.localizeImages(doc, URL, path)

	var sb strings.Builder

	// problem title
	title := strings.TrimSpace(doc.Find(".title").First().Text())
	if title != "" {
		sb.WriteString("# ")
		sb.WriteString(title)
		sb.WriteString("\n\n")
	}

	// time / memory limits
	limits := strings.TrimSpace(doc.Find(".time-limit").Text())
	if limits != "" {
		sb.WriteString("**")
		sb.WriteString(strings.TrimSpace(doc.Find(".property-title").First().Text()))
		sb.WriteString("** ")
		doc.Find(".time-limit .property-title").Remove()
		sb.WriteString(strings.TrimSpace(doc.Find(".time-limit").Text()))
		sb.WriteString("\n")
		_ = limits
	}
	sb.WriteString("\n")
	memLimit := strings.TrimSpace(doc.Find(".memory-limit").Text())
	if memLimit != "" {
		sb.WriteString("**")
		sb.WriteString(strings.TrimSpace(doc.Find(".memory-limit .property-title").Text()))
		sb.WriteString("** ")
		doc.Find(".memory-limit .property-title").Remove()
		sb.WriteString(strings.TrimSpace(doc.Find(".memory-limit").Text()))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// main statement sections (problem-statement children divs)
	visited := make(map[*html.Node]bool)
	doc.Find(".problem-statement > div").Each(func(_ int, s *goquery.Selection) {
		// skip header div (title/limits already handled)
		if s.HasClass("header") {
			return
		}
		// sample test blocks — render as plain text tables
		if s.HasClass("sample-tests") {
			sb.WriteString("## Sample Tests\n\n")
			s.Find(".sample-test").Each(func(i int, st *goquery.Selection) {
				sb.WriteString(fmt.Sprintf("### Sample %d\n\n", i+1))
				inputText := strings.TrimSpace(nodeText(st.Find(".input pre"), visited))
				reg := regexp.MustCompile(`\n{2,}`)
				inputText = reg.ReplaceAllString(inputText, "\n")
				outputText := strings.TrimSpace(nodeText(st.Find(".output pre"), visited))
				outputText = reg.ReplaceAllString(outputText, "\n")
				sb.WriteString("**Input**\n```\n")
				sb.WriteString(inputText)
				sb.WriteString("\n```\n\n**Output**\n```\n")
				sb.WriteString(outputText)
				sb.WriteString("\n```\n\n")
			})
			return
		}
		// note / other sections
		sb.WriteString(sectionText(s, visited))
	})

	doc.Find(".ttypography").First().Children().Each(func(_ int, s *goquery.Selection) {
		if goquery.NodeName(s) == "div" {
			if s.HasClass("problem-statement") {
				return
			}
			if s.HasClass("spoiler") {
				sb.WriteString(c.renderSpoiler(s, csrf, URL, visited))
				return
			}
		}
		sb.WriteString(nodeText(s, visited))
		sb.WriteString("\n\n")
	})

	content := sb.String()
	// trim excessive blank lines globally
	reg := regexp.MustCompile(`\n{3,}`)
	content = reg.ReplaceAllString(content, "\n\n")
	// Codeforces wraps math in $$$ (inline) and $$$$$$ (display). Collapse
	// them to $...$ / $$...$$ with the delimiters tight against the
	// expression: GitHub, VS Code and most KaTeX-based renderers only treat
	// $...$ as math when no whitespace touches the dollar signs.
	content = strings.ReplaceAll(content, "$$$", "$")

	outFile := filepath.Join(path, fileName)
	if err := ioutil.WriteFile(outFile, []byte(content), 0644); err != nil {
		return err
	}
	return nil
}
