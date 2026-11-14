package patreon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderContentJSON(t *testing.T) {
	doc := `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Running order"}]},
		{"type":"paragraph","content":[
			{"type":"text","text":"Here are this month's "},
			{"type":"text","marks":[{"type":"link","attrs":{"href":"https://www.patreon.com/collection/1349741"}},{"type":"underline"}],"text":"Bonus Bits"},
			{"type":"text","text":":"}]},
		{"type":"orderedList","attrs":{"start":3},"content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","marks":[{"type":"bold"}],"text":"Pets"}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Ghosts"}]}]}]},
		{"type":"horizontalRule"},
		{"type":"paragraph"},
		{"type":"bulletList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"One"}]}]}]},
		{"type":"paragraph","content":[
			{"type":"text","text":"Line one"},{"type":"hardBreak"},{"type":"text","text":"Line two "},
			{"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.com"}}],"text":"https://example.com"}]}
	]}`

	expected := "Running order\n\n" +
		"Here are this month's Bonus Bits (https://www.patreon.com/collection/1349741):\n\n" +
		"3. Pets\n4. Ghosts\n\n" +
		"- One\n\n" +
		"Line one\nLine two https://example.com"

	assert.Equal(t, expected, renderContentJSON(doc))
}

func TestRenderContentJSON_OtherBlocks(t *testing.T) {
	doc := `{"type":"doc","content":[
		{"type":"caption","content":[{"type":"text","text":"Taken "},{"type":"text","marks":[{"type":"italic"}],"text":"yesterday"}]},
		{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"Quoted"}]}]},
		{"type":"bulletList","content":[{"type":"listItem","content":[
			{"type":"paragraph","content":[{"type":"text","text":"Outer"}]},
			{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Inner"}]}]}]}]}]},
		{"type":"cta","attrs":{"button_text":"Shop the calendar","button_link":"https://shop.example.com/","caption":"Pre-orders are open!"}}
	]}`

	expected := "Taken yesterday\n\n" +
		"Quoted\n\n" +
		"- Outer\n- Inner\n\n" +
		"Pre-orders are open! Shop the calendar (https://shop.example.com/)"

	assert.Equal(t, expected, renderContentJSON(doc))
}

func TestRenderContentJSON_Media(t *testing.T) {
	doc := `{"type":"doc","content":[
		{"type":"image","attrs":{"src":"https://img.itch.zone/original/x.gif","alt":null,"caption":null}},
		{"type":"image","attrs":{"src":"https://c10.patreonusercontent.com/1.png?token-hash=abc&token-time=1795910400","alt":"Map of the tour","caption":null}},
		{"type":"image","attrs":{"src":"https://img/captioned.png","alt":"ignored","caption":"Our new\n  cat "}},
		{"type":"video","attrs":{"fallback_strategy":"fallback","media_id":"781531560"}},
		{"type":"paragraph","content":[
			{"type":"text","text":"Before"},
			{"type":"image","attrs":{"src":"https://img/inline.png"}},
			{"type":"text","text":"After"}]},
		{"type":"paragraph","content":[{"type":"image","attrs":{"src":"https://img/alone.png"}}]}
	]}`

	expected := "[Image] (https://img.itch.zone/original/x.gif)\n\n" +
		"[Image: Map of the tour]\n\n" +
		"[Image: Our new cat] (https://img/captioned.png)\n\n" +
		"[Video]\n\n" +
		"Before\n[Image] (https://img/inline.png)\nAfter\n\n" +
		"[Image] (https://img/alone.png)"

	assert.Equal(t, expected, renderContentJSON(doc))
}

func TestRenderContentJSON_Invalid(t *testing.T) {
	assert.Equal(t, "", renderContentJSON(""))
	assert.Equal(t, "", renderContentJSON("not json"))
}
