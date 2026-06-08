package ui

import (
	"regexp"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

var bareURLRe = regexp.MustCompile(`https?://[^\s\)\]>\"'<\]]+`)

// autolinkBareURLs wraps bare http(s) URLs in markdown link syntax so glamour
// renders them as OSC-8 hyperlinks in the terminal.
func autolinkBareURLs(md string) string {
	return bareURLRe.ReplaceAllStringFunc(md, func(match string) string {
		url := strings.TrimRight(match, ".,;:!?")
		if url == "" {
			return match
		}
		return "[" + url + "](" + url + ")"
	})
}

// chatURLAt returns the URL under the given visual column on a rendered chat
// line, checking OSC-8 hyperlink regions first and falling back to bare URLs.
func chatURLAt(line string, cellX int) string {
	if cellX < 0 {
		return ""
	}
	if url := hyperlinkAtCol(line, cellX); url != "" {
		return url
	}
	plain := ansi.Strip(line)
	for _, loc := range bareURLRe.FindAllStringIndex(plain, -1) {
		start, end := loc[0], loc[1]
		if cellX >= start && cellX < end {
			return strings.TrimRight(plain[start:end], ".,;:!?")
		}
	}
	return ""
}

func hyperlinkAtCol(s string, targetCol int) string {
	var activeURL string
	col := 0
	i := 0
	for i < len(s) {
		if i+3 < len(s) && s[i] == '\x1b' && s[i+1] == ']' && s[i+2] == '8' && s[i+3] == ';' {
			end := strings.Index(s[i:], "\x07")
			if end < 0 {
				break
			}
			seq := s[i : i+end+1]
			activeURL = parseOSC8URI(seq)
			i += end + 1
			continue
		}
		if i+1 < len(s) && s[i] == '\x1b' && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		w := runewidth.RuneWidth(r)
		if col <= targetCol && targetCol < col+w && activeURL != "" {
			return activeURL
		}
		col += w
		i += size
	}
	return ""
}

func parseOSC8URI(seq string) string {
	if len(seq) < 6 || !strings.HasPrefix(seq, "\x1b]8;") || seq[len(seq)-1] != '\x07' {
		return ""
	}
	content := seq[4 : len(seq)-1]
	semi := strings.LastIndex(content, ";")
	if semi < 0 {
		return ""
	}
	return content[semi+1:]
}

// handleChatLinkClick opens the URL under the cursor when invoked with Ctrl held.
func (m *Model) handleChatLinkClick(x, y int) tea.Cmd {
	if !m.mouseInChatInner(x, y) {
		return nil
	}
	sess := m.currentSession()
	if sess == nil {
		return nil
	}
	lineIdx, cellX, ok := m.mouseToChatCell(x, y)
	if !ok {
		return nil
	}
	lineIdx = m.clampChatLineIndex(sess, lineIdx)
	lines := m.visibleChatLines(sess, m.currentLayout())
	if lineIdx < 0 || lineIdx >= len(lines) {
		return nil
	}
	url := chatURLAt(lines[lineIdx], cellX)
	if url == "" {
		return nil
	}
	if err := openBrowser(url); err != nil {
		return m.emitStatusMsg("Could not open link: "+err.Error(), StatusMsgError)
	}
	return m.emitStatusMsg("Opened link in browser", StatusMsgInfo)
}