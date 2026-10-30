// Package adapter translates provider-native request bodies into the hosted
// Next Moca contract and can safely reinsert the server-selected original text.
package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/nextmoca/envoy-nextmoca-runtime-defense/internal/nextmoca"
)

const OmissionMarker = "[Next Moca omitted nonessential context for this request.]"

type Request struct {
	Adapter    string
	TaskPrompt string
	Records    []nextmoca.Record
	body       map[string]any
	candidates []candidate
}

type candidate struct {
	record nextmoca.Record
	set    func(map[string]any, string) error
}

func Parse(raw []byte, headers http.Header, requested string) (Request, error) {
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return Request{}, fmt.Errorf("invalid JSON request: %w", err)
	}
	if body == nil {
		return Request{}, errors.New("request body must be a JSON object")
	}

	adapterName, err := resolveAdapter(body, headers, requested)
	if err != nil {
		return Request{}, err
	}
	switch adapterName {
	case "openai_chat":
		return parseOpenAIChat(body)
	case "openai_responses":
		return parseOpenAIResponses(body)
	case "anthropic_messages":
		return parseAnthropicMessages(body)
	case "gemini_generate_content":
		return parseGeminiGenerateContent(body)
	default:
		return Request{}, fmt.Errorf("unsupported request adapter %q", adapterName)
	}
}

func (r Request) CandidateCount() int { return len(r.candidates) }

func (r Request) HasCandidate(id string) bool {
	for _, candidate := range r.candidates {
		if candidate.record.ID == id {
			return true
		}
	}
	return false
}

func (r Request) RewriteSelected(excerpts map[string]string) ([]byte, error) {
	if len(r.candidates) == 0 {
		return nil, errors.New("request has no candidate records")
	}
	clone, err := cloneBody(r.body)
	if err != nil {
		return nil, err
	}
	for _, candidate := range r.candidates {
		value := OmissionMarker
		if excerpt := strings.TrimSpace(excerpts[candidate.record.ID]); excerpt != "" {
			value = excerpt
		}
		if err := candidate.set(clone, value); err != nil {
			return nil, err
		}
	}
	return json.Marshal(clone)
}

func (r Request) RewriteSingleCandidate(rendered string) ([]byte, error) {
	if len(r.candidates) != 1 {
		return nil, errors.New("sanitized fallback requires exactly one candidate record")
	}
	if strings.TrimSpace(rendered) == "" {
		return nil, errors.New("sanitized fallback is empty")
	}
	return r.RewriteSelected(map[string]string{r.candidates[0].record.ID: rendered})
}

func resolveAdapter(body map[string]any, headers http.Header, requested string) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested != "" && requested != "auto" {
		return requested, nil
	}
	if _, ok := body["input"]; ok {
		return "openai_responses", nil
	}
	if _, ok := body["contents"]; ok {
		return "gemini_generate_content", nil
	}
	if _, ok := body["messages"]; ok && headers != nil && headers.Get("anthropic-version") != "" {
		return "anthropic_messages", nil
	}
	if _, ok := body["messages"]; ok {
		return "", errors.New("ambiguous messages request: set request_adapter to openai_chat or anthropic_messages")
	}
	return "", errors.New("unsupported request shape")
}

func parseOpenAIChat(body map[string]any) (Request, error) {
	messages, ok := arrayField(body, "messages")
	if !ok {
		return Request{}, errors.New("openai_chat requires messages array")
	}
	taskIndex, parsed, err := parseStringMessages(messages, "openai.messages", map[string]messageSpec{
		"user": {kind: "user_input", sourceKind: "user"}, "assistant": {kind: "llm_response", sourceKind: "unknown"}, "tool": {kind: "tool_result", sourceKind: "tool"},
	})
	if err != nil {
		return Request{}, err
	}
	return makeMessageRequest("openai_chat", body, parsed, taskIndex, func(copy map[string]any, index int, value string) error {
		messages, _ := arrayField(copy, "messages")
		message, ok := object(messages[index])
		if !ok {
			return errors.New("rewritten OpenAI message is invalid")
		}
		message["content"] = value
		return nil
	})
}

func parseAnthropicMessages(body map[string]any) (Request, error) {
	messages, ok := arrayField(body, "messages")
	if !ok {
		return Request{}, errors.New("anthropic_messages requires messages array")
	}
	taskIndex, parsed, err := parseStringMessages(messages, "anthropic.messages", map[string]messageSpec{
		"user": {kind: "user_input", sourceKind: "user"}, "assistant": {kind: "llm_response", sourceKind: "unknown"},
	})
	if err != nil {
		return Request{}, err
	}
	return makeMessageRequest("anthropic_messages", body, parsed, taskIndex, func(copy map[string]any, index int, value string) error {
		messages, _ := arrayField(copy, "messages")
		message, ok := object(messages[index])
		if !ok {
			return errors.New("rewritten Anthropic message is invalid")
		}
		message["content"] = value
		return nil
	})
}

func parseOpenAIResponses(body map[string]any) (Request, error) {
	input, exists := body["input"]
	if !exists {
		return Request{}, errors.New("openai_responses requires input")
	}
	if prompt, ok := input.(string); ok && strings.TrimSpace(prompt) != "" {
		return Request{Adapter: "openai_responses", TaskPrompt: prompt, body: body}, nil
	}
	items, ok := input.([]any)
	if !ok {
		return Request{}, errors.New("openai_responses input must be a string or array")
	}
	taskIndex := -1
	type item struct {
		role, content, kind, sourceKind string
		candidate                       bool
	}
	parsed := make([]item, len(items))
	for index, value := range items {
		objectValue, ok := object(value)
		if !ok {
			return Request{}, fmt.Errorf("openai input[%d] is not an object", index)
		}
		itemType, _ := objectValue["type"].(string)
		if itemType == "function_call_output" {
			output, ok := objectValue["output"].(string)
			if !ok {
				return Request{}, fmt.Errorf("openai input[%d] function output must be a string", index)
			}
			parsed[index] = item{content: output, kind: "tool_result", sourceKind: "tool", candidate: true}
			continue
		}
		if itemType != "" && itemType != "message" {
			continue
		}
		role, ok := objectValue["role"].(string)
		if !ok {
			return Request{}, fmt.Errorf("openai input[%d] has no string role", index)
		}
		content, ok := objectValue["content"].(string)
		if !ok {
			return Request{}, fmt.Errorf("openai input[%d] content must be a string", index)
		}
		spec, candidate := map[string]messageSpec{"user": {"user_input", "user"}, "assistant": {"llm_response", "unknown"}}[role]
		parsed[index] = item{role: role, content: content, kind: spec.kind, sourceKind: spec.sourceKind, candidate: candidate}
		if role == "user" && strings.TrimSpace(content) != "" {
			taskIndex = index
		}
	}
	if taskIndex < 0 {
		return Request{}, errors.New("openai_responses has no user task")
	}
	request := Request{Adapter: "openai_responses", TaskPrompt: parsed[taskIndex].content, body: body}
	for index := 0; index < taskIndex; index++ {
		if !parsed[index].candidate || strings.TrimSpace(parsed[index].content) == "" {
			continue
		}
		candidateIndex := index
		record := nextmoca.Record{ID: fmt.Sprintf("input-%d", index), Kind: parsed[index].kind, Source: fmt.Sprintf("openai.input[%d]", index), SourceKind: parsed[index].sourceKind, Text: parsed[index].content}
		request.Records = append(request.Records, record)
		request.candidates = append(request.candidates, candidate{record: record, set: func(copy map[string]any, replacement string) error {
			items, _ := arrayField(copy, "input")
			item, ok := object(items[candidateIndex])
			if !ok {
				return errors.New("rewritten OpenAI Responses item is invalid")
			}
			if item["type"] == "function_call_output" {
				item["output"] = replacement
			} else {
				item["content"] = replacement
			}
			return nil
		}})
	}
	return request, nil
}

func parseGeminiGenerateContent(body map[string]any) (Request, error) {
	contents, ok := arrayField(body, "contents")
	if !ok {
		return Request{}, errors.New("gemini_generate_content requires contents array")
	}
	taskIndex := -1
	task := ""
	type contentItem struct {
		role  string
		parts []any
	}
	parsed := make([]contentItem, len(contents))
	for index, value := range contents {
		content, ok := object(value)
		if !ok {
			return Request{}, fmt.Errorf("gemini contents[%d] is not an object", index)
		}
		role, ok := content["role"].(string)
		if !ok {
			return Request{}, fmt.Errorf("gemini contents[%d] has no role", index)
		}
		parts, ok := arrayField(content, "parts")
		if !ok || len(parts) == 0 {
			return Request{}, fmt.Errorf("gemini contents[%d] has no parts", index)
		}
		for partIndex, part := range parts {
			partObject, ok := object(part)
			if !ok {
				return Request{}, fmt.Errorf("gemini contents[%d].parts[%d] is invalid", index, partIndex)
			}
			if _, ok := partObject["text"].(string); !ok {
				return Request{}, fmt.Errorf("gemini contents[%d].parts[%d] is not text", index, partIndex)
			}
		}
		parsed[index] = contentItem{role: role, parts: parts}
		if role == "user" {
			texts := partTexts(parts)
			if strings.TrimSpace(strings.Join(texts, "\n")) != "" {
				taskIndex, task = index, strings.Join(texts, "\n")
			}
		}
	}
	if taskIndex < 0 {
		return Request{}, errors.New("gemini_generate_content has no user task")
	}
	request := Request{Adapter: "gemini_generate_content", TaskPrompt: task, body: body}
	for contentIndex := 0; contentIndex < taskIndex; contentIndex++ {
		role := parsed[contentIndex].role
		if role != "user" && role != "model" {
			continue
		}
		kind, sourceKind := "user_input", "user"
		if role == "model" {
			kind, sourceKind = "llm_response", "unknown"
		}
		for partIndex, part := range parsed[contentIndex].parts {
			text := objectMust(part)["text"].(string)
			if strings.TrimSpace(text) == "" {
				continue
			}
			contentPosition, partPosition := contentIndex, partIndex
			record := nextmoca.Record{ID: fmt.Sprintf("content-%d-part-%d", contentIndex, partIndex), Kind: kind, Source: fmt.Sprintf("gemini.contents[%d].parts[%d]", contentIndex, partIndex), SourceKind: sourceKind, Text: text}
			request.Records = append(request.Records, record)
			request.candidates = append(request.candidates, candidate{record: record, set: func(copy map[string]any, replacement string) error {
				contents, _ := arrayField(copy, "contents")
				content, ok := object(contents[contentPosition])
				if !ok {
					return errors.New("rewritten Gemini content is invalid")
				}
				parts, _ := arrayField(content, "parts")
				part, ok := object(parts[partPosition])
				if !ok {
					return errors.New("rewritten Gemini part is invalid")
				}
				part["text"] = replacement
				return nil
			}})
		}
	}
	return request, nil
}

type messageSpec struct{ kind, sourceKind string }
type stringMessage struct {
	role, content, kind, sourceKind string
	candidate                       bool
}

func parseStringMessages(messages []any, source string, specs map[string]messageSpec) (int, []stringMessage, error) {
	taskIndex := -1
	parsed := make([]stringMessage, len(messages))
	for index, value := range messages {
		message, ok := object(value)
		if !ok {
			return -1, nil, fmt.Errorf("%s[%d] is not an object", source, index)
		}
		role, ok := message["role"].(string)
		if !ok {
			return -1, nil, fmt.Errorf("%s[%d] has no string role", source, index)
		}
		content, ok := message["content"].(string)
		if !ok {
			return -1, nil, fmt.Errorf("%s[%d] content must be a string", source, index)
		}
		spec, candidate := specs[role]
		parsed[index] = stringMessage{role: role, content: content, kind: spec.kind, sourceKind: spec.sourceKind, candidate: candidate}
		if role == "user" && strings.TrimSpace(content) != "" {
			taskIndex = index
		}
	}
	if taskIndex < 0 {
		return -1, nil, fmt.Errorf("%s has no user task", source)
	}
	return taskIndex, parsed, nil
}

func makeMessageRequest(adapterName string, body map[string]any, parsed []stringMessage, taskIndex int, setter func(map[string]any, int, string) error) (Request, error) {
	request := Request{Adapter: adapterName, TaskPrompt: parsed[taskIndex].content, body: body}
	for index := 0; index < taskIndex; index++ {
		message := parsed[index]
		if !message.candidate || strings.TrimSpace(message.content) == "" {
			continue
		}
		position := index
		record := nextmoca.Record{ID: fmt.Sprintf("message-%d", index), Kind: message.kind, Source: fmt.Sprintf("%s.messages[%d]", adapterName, index), SourceKind: message.sourceKind, Text: message.content}
		request.Records = append(request.Records, record)
		request.candidates = append(request.candidates, candidate{record: record, set: func(copy map[string]any, value string) error { return setter(copy, position, value) }})
	}
	return request, nil
}

func cloneBody(body map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var clone map[string]any
	if err := json.Unmarshal(raw, &clone); err != nil {
		return nil, err
	}
	return clone, nil
}

func arrayField(objectValue map[string]any, key string) ([]any, bool) {
	value, ok := objectValue[key].([]any)
	return value, ok
}
func object(value any) (map[string]any, bool) {
	result, ok := value.(map[string]any)
	return result, ok
}
func objectMust(value any) map[string]any { result, _ := object(value); return result }
func partTexts(parts []any) []string {
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		texts = append(texts, objectMust(part)["text"].(string))
	}
	return texts
}
