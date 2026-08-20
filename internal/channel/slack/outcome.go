package slack

import "strings"

/*
outcome turns the model's Markdown-ish answer into Slack's mrkdwn subset.

Small on purpose. Slack's format is close enough to Markdown to tempt a raw
pass-through and different enough to make the result ugly. The conversion is
also a trust boundary: links show their destination, images never render, and
Slack's active angle-bracket forms stay escaped unless this code writes them.
*/
func outcome(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			out = append(out, "```")
			inFence = !inFence
			continue
		}
		if inFence {
			out = append(out, escape(line))
			continue
		}
		if heading, ok := heading(line); ok {
			out = append(out, "*"+inline(heading)+"*")
			continue
		}
		out = append(out, inline(line))
	}
	return strings.Join(out, "\n")
}

func heading(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	n := 0
	for n < len(trimmed) && n < 6 && trimmed[n] == '#' {
		n++
	}
	if n == 0 || n >= len(trimmed) || trimmed[n] != ' ' {
		return "", false
	}
	return strings.TrimSpace(trimmed[n+1:]), true
}

func inline(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); {
		switch {
		case strings.HasPrefix(text[i:], "`"):
			end := strings.Index(text[i+1:], "`")
			if end < 0 {
				out.WriteByte('`')
				i++
				continue
			}
			code := text[i+1 : i+1+end]
			out.WriteByte('`')
			out.WriteString(escape(code))
			out.WriteByte('`')
			i += end + 2
		case strings.HasPrefix(text[i:], "!["):
			label, url, end, ok := markdownLink(text, i+1)
			if !ok {
				out.WriteByte(text[i])
				i++
				continue
			}
			if strings.TrimSpace(label) != "" {
				out.WriteString(escape(label))
				out.WriteString(" ")
			}
			out.WriteString("(image: ")
			out.WriteString(escape(url))
			out.WriteByte(')')
			i = end
		case strings.HasPrefix(text[i:], "["):
			label, url, end, ok := markdownLink(text, i)
			if !ok {
				out.WriteByte(text[i])
				i++
				continue
			}
			out.WriteString(escape(label))
			out.WriteString(" (")
			out.WriteString(escape(url))
			out.WriteByte(')')
			i = end
		case strings.HasPrefix(text[i:], "**"):
			end := strings.Index(text[i+2:], "**")
			if end < 0 {
				out.WriteString("**")
				i += 2
				continue
			}
			out.WriteByte('*')
			out.WriteString(inline(text[i+2 : i+2+end]))
			out.WriteByte('*')
			i += end + 4
		default:
			switch text[i] {
			case '&':
				out.WriteString("&amp;")
			case '<':
				out.WriteString("&lt;")
			case '>':
				out.WriteString("&gt;")
			default:
				out.WriteByte(text[i])
			}
			i++
		}
	}
	return out.String()
}

func markdownLink(text string, start int) (label, url string, end int, ok bool) {
	if start >= len(text) || text[start] != '[' {
		return "", "", start, false
	}
	closeLabel := strings.IndexByte(text[start+1:], ']')
	if closeLabel < 0 {
		return "", "", start, false
	}
	closeLabel += start + 1
	if closeLabel+1 >= len(text) || text[closeLabel+1] != '(' {
		return "", "", start, false
	}
	closeURL := strings.IndexByte(text[closeLabel+2:], ')')
	if closeURL < 0 {
		return "", "", start, false
	}
	closeURL += closeLabel + 2
	return text[start+1 : closeLabel], text[closeLabel+2 : closeURL], closeURL + 1, true
}
