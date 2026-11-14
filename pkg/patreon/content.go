package patreon

import (
	"encoding/json"
	"strconv"
	"strings"
)

// contentNode is a node of the ProseMirror document Patreon stores in a post's content_json_string.
type contentNode struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Attrs   json.RawMessage `json:"attrs"`
	Marks   []contentMark   `json:"marks"`
	Content []contentNode   `json:"content"`
}

type contentMark struct {
	Type  string `json:"type"`
	Attrs struct {
		Href string `json:"href"`
	} `json:"attrs"`
}

func renderContentJSON(s string) string {
	if s == "" {
		return ""
	}

	var doc contentNode
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		return ""
	}

	return strings.TrimSpace(renderBlocks(doc.Content, "\n\n"))
}

func renderBlocks(nodes []contentNode, sep string) string {
	parts := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if text := strings.TrimSpace(renderBlock(node)); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, sep)
}

func renderBlock(node contentNode) string {
	switch node.Type {
	case "paragraph", "heading", "caption":
		return renderInline(node.Content)
	case "cta":
		return renderCallToAction(node)
	case "orderedList", "bulletList":
		return renderList(node)
	case "image", "video":
		return renderMedia(node)
	case "horizontalRule":
		return ""
	default:
		if node.Text != "" {
			return renderInline([]contentNode{node})
		}
		return renderBlocks(node.Content, "\n\n")
	}
}

func renderList(list contentNode) string {
	number := 1
	if list.Type == "orderedList" && len(list.Attrs) > 0 {
		var attrs struct {
			Start *int `json:"start"`
		}
		if err := json.Unmarshal(list.Attrs, &attrs); err == nil && attrs.Start != nil {
			number = *attrs.Start
		}
	}

	items := make([]string, 0, len(list.Content))
	for _, item := range list.Content {
		text := renderBlocks(item.Content, "\n")
		if text == "" {
			continue
		}

		prefix := "- "
		if list.Type == "orderedList" {
			prefix = strconv.Itoa(number) + ". "
			number++
		}
		items = append(items, prefix+text)
	}

	return strings.Join(items, "\n")
}

func renderMedia(node contentNode) string {
	var attrs struct {
		Src     string `json:"src"`
		Alt     string `json:"alt"`
		Caption string `json:"caption"`
	}
	_ = json.Unmarshal(node.Attrs, &attrs)

	label := "Image"
	if node.Type == "video" {
		label = "Video"
	}
	if description := firstNonBlank(attrs.Caption, attrs.Alt); description != "" {
		label += ": " + description
	}

	// Patreon-hosted media URLs are signed with a token-time that expires within weeks; inline videos carry
	// only a media_id that the posts API doesn't resolve.
	if attrs.Src == "" || strings.Contains(attrs.Src, "token-time=") {
		return "[" + label + "]"
	}
	return "[" + label + "] (" + attrs.Src + ")"
}

func renderInline(nodes []contentNode) string {
	var sb strings.Builder
	for i, node := range nodes {
		switch node.Type {
		case "hardBreak":
			sb.WriteString("\n")
		case "text":
			sb.WriteString(node.Text)
			if href := linkHref(node.Marks); href != "" && href != node.Text {
				sb.WriteString(" (" + href + ")")
			}
		case "image", "video":
			if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString(renderMedia(node))
			if i < len(nodes)-1 {
				sb.WriteString("\n")
			}
		default:
			sb.WriteString(renderInline(node.Content))
		}
	}
	return sb.String()
}

func renderCallToAction(node contentNode) string {
	var attrs struct {
		Caption    string `json:"caption"`
		ButtonText string `json:"button_text"`
		ButtonLink string `json:"button_link"`
	}
	if err := json.Unmarshal(node.Attrs, &attrs); err != nil {
		return ""
	}

	parts := make([]string, 0, 3)
	for _, part := range []string{attrs.Caption, attrs.ButtonText} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if attrs.ButtonLink != "" {
		parts = append(parts, "("+attrs.ButtonLink+")")
	}

	return strings.Join(parts, " ")
}

func linkHref(marks []contentMark) string {
	for _, mark := range marks {
		if mark.Type == "link" {
			return mark.Attrs.Href
		}
	}
	return ""
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if v = strings.Join(strings.Fields(v), " "); v != "" {
			return v
		}
	}
	return ""
}
