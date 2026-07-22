package convert

import (
	"strings"

	"github.com/lich0821/ccNexus/internal/transformer"
)

const (
	thinkTagOpen     = "<think>"
	thinkTagClose    = "</think>"
	thinkingTagOpen  = "<thinking>"
	thinkingTagClose = "</thinking>"
)

var thinkTags = []struct {
	open  string
	close string
}{
	{open: thinkTagOpen, close: thinkTagClose},
	{open: thinkingTagOpen, close: thinkingTagClose},
}

func splitThinkTaggedText(text string) []map[string]interface{} {
	var blocks []map[string]interface{}
	for {
		openIdx, _, closeTag := findOpeningThinkTag(text)
		if openIdx == -1 {
			if text != "" {
				blocks = append(blocks, map[string]interface{}{"type": "text", "text": text})
			}
			return blocks
		}
		if openIdx > 0 {
			blocks = append(blocks, map[string]interface{}{"type": "text", "text": text[:openIdx]})
		}
		_, openTag, _ := findOpeningThinkTag(text[openIdx:])
		text = text[openIdx+len(openTag):]
		closeIdx := strings.Index(text, closeTag)
		if closeIdx == -1 {
			if text != "" {
				blocks = append(blocks, map[string]interface{}{"type": "text", "text": text})
			}
			return blocks
		}
		if closeIdx > 0 {
			blocks = append(blocks, map[string]interface{}{"type": "thinking", "thinking": text[:closeIdx]})
		}
		text = text[closeIdx+len(closeTag):]
	}
}

func consumeThinkTaggedStream(content string, ctx *transformer.StreamContext, emitText func(string), emitThinking func(string)) {
	for len(content) > 0 {
		if ctx.InThinkingTag {
			closeTag := ctx.ThinkingTagClose
			if closeTag == "" {
				closeTag = thinkTagClose
			}
			closeIdx := strings.Index(content, closeTag)
			if closeIdx == -1 {
				text, buffer := splitTrailingPartialTag(content, closeTag)
				if text != "" {
					emitThinking(text)
				}
				ctx.ThinkingBuffer = buffer
				return
			}
			if closeIdx > 0 {
				emitThinking(content[:closeIdx])
			}
			ctx.InThinkingTag = false
			ctx.ThinkingTagClose = ""
			content = content[closeIdx+len(closeTag):]
			continue
		}

		openIdx, openTag, closeTag := findOpeningThinkTag(content)
		if openIdx == -1 {
			text, buffer := splitTrailingPartialOpeningThinkTag(content)
			emitText(text)
			ctx.ThinkingBuffer = buffer
			return
		}
		emitText(content[:openIdx])
		ctx.InThinkingTag = true
		ctx.ThinkingTagClose = closeTag
		content = content[openIdx+len(openTag):]
	}
}

func flushThinkTaggedStream(ctx *transformer.StreamContext, emitText func(string), emitThinking func(string)) {
	if ctx.InThinkingTag {
		if ctx.ThinkingBuffer != "" {
			emitThinking(ctx.ThinkingBuffer)
		}
	} else if ctx.ThinkingBuffer != "" {
		emitText(ctx.ThinkingBuffer)
	}
	ctx.InThinkingTag = false
	ctx.ThinkingBuffer = ""
	ctx.ThinkingTagClose = ""
}

func findOpeningThinkTag(s string) (int, string, string) {
	bestIdx := -1
	var bestOpen, bestClose string
	for _, tags := range thinkTags {
		idx := strings.Index(s, tags.open)
		if idx != -1 && (bestIdx == -1 || idx < bestIdx) {
			bestIdx = idx
			bestOpen = tags.open
			bestClose = tags.close
		}
	}
	return bestIdx, bestOpen, bestClose
}

func splitTrailingPartialOpeningThinkTag(s string) (string, string) {
	bestText, bestBuffer := s, ""
	for _, tags := range thinkTags {
		text, buffer := splitTrailingPartialTag(s, tags.open)
		if len(buffer) > len(bestBuffer) {
			bestText = text
			bestBuffer = buffer
		}
	}
	return bestText, bestBuffer
}

func makeThinkEmitters(ctx *transformer.StreamContext, result *[]byte) (func(string), func(string)) {
	emitText := func(text string) {
		if text == "" {
			return
		}
		if !ctx.ContentBlockStarted {
			ctx.ContentBlockStarted = true
			*result = append(*result, buildClaudeEvent("content_block_start", map[string]interface{}{
				"index": ctx.ContentIndex, "content_block": map[string]interface{}{"type": "text", "text": ""},
			})...)
		}
		*result = append(*result, buildClaudeEvent("content_block_delta", map[string]interface{}{
			"index": ctx.ContentIndex, "delta": map[string]interface{}{"type": "text_delta", "text": text},
		})...)
	}

	emitThinking := func(text string) {
		if text == "" {
			return
		}
		if !ctx.ThinkingBlockStarted {
			if ctx.ContentBlockStarted {
				*result = append(*result, buildClaudeEvent("content_block_stop", map[string]interface{}{"index": ctx.ContentIndex})...)
				ctx.ContentBlockStarted = false
				ctx.ContentIndex++
			}
			ctx.ThinkingBlockStarted = true
			ctx.ThinkingIndex = ctx.ContentIndex
			ctx.ContentIndex++
			*result = append(*result, buildClaudeEvent("content_block_start", map[string]interface{}{
				"index": ctx.ThinkingIndex, "content_block": map[string]interface{}{"type": "thinking", "thinking": ""},
			})...)
		}
		*result = append(*result, buildClaudeEvent("content_block_delta", map[string]interface{}{
			"index": ctx.ThinkingIndex, "delta": map[string]interface{}{"type": "thinking_delta", "thinking": text},
		})...)
	}

	return emitText, emitThinking
}

func splitTrailingPartialTag(s, tag string) (string, string) {
	if s == "" || tag == "" {
		return s, ""
	}
	max := len(tag) - 1
	if max > len(s) {
		max = len(s)
	}
	for i := max; i > 0; i-- {
		if strings.HasPrefix(tag, s[len(s)-i:]) {
			return s[:len(s)-i], s[len(s)-i:]
		}
	}
	return s, ""
}
