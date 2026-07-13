package openai

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

type responsesEventRepairer struct {
	state *ResponsesStreamState
}

func newResponsesEventRepairer() *responsesEventRepairer {
	return &responsesEventRepairer{state: newResponsesStreamState()}
}

func (r *responsesEventRepairer) repair(payload []byte) [][]byte {
	if r == nil {
		return [][]byte{payload}
	}
	if r.state == nil {
		r.state = newResponsesStreamState()
	}

	switch strings.TrimSpace(gjson.GetBytes(payload, "type").String()) {
	case "response.output_item.added":
		r.state.recordOutputItemAdded(payload)
	case "response.content_part.added":
		r.state.recordContentPartAdded(payload)
	case "response.output_item.done":
		r.state.recordOutputItemDone(payload)
	case "response.output_text.delta":
		return r.repairOutputTextDelta(payload)
	case "response.completed":
		repaired := r.state.repairCompletedPayload(payload)
		r.state.observeSequence(payload)
		r.state.close()
		return [][]byte{repaired}
	}

	r.state.observeSequence(payload)
	return [][]byte{payload}
}

func (r *responsesEventRepairer) repairOutputTextDelta(payload []byte) [][]byte {
	itemID := gjson.GetBytes(payload, "item_id").String()
	if strings.TrimSpace(itemID) == "" {
		r.state.observeSequence(payload)
		return [][]byte{payload}
	}

	contentIndex := 0
	if result := gjson.GetBytes(payload, "content_index"); result.Exists() {
		contentIndex = int(result.Int())
	}
	item := r.state.Items[itemID]
	if item != nil && item.Type != "" && item.Type != "message" {
		r.state.observeSequence(payload)
		return [][]byte{payload}
	}

	needsItem := item == nil || !item.Added
	needsContentPart := !r.state.hasContentPart(itemID, contentIndex)
	syntheticCount := 0
	if needsItem {
		syntheticCount++
	}
	if needsContentPart {
		syntheticCount++
	}
	if syntheticCount == 0 {
		r.state.observeSequence(payload)
		return [][]byte{payload}
	}

	sequences := r.state.allocateSyntheticSequences(syntheticCount, payload)
	events := make([][]byte, 0, syntheticCount+1)
	sequenceIndex := 0
	if needsItem {
		added := syntheticOutputItemAdded(payload, sequences[sequenceIndex])
		sequenceIndex++
		events = append(events, added)
		r.state.recordOutputItemAdded(added)
	}
	if needsContentPart {
		added := syntheticContentPartAdded(payload, sequences[sequenceIndex])
		events = append(events, added)
		r.state.recordContentPartAdded(added)
	}

	events = append(events, payload)
	r.state.observeSequence(payload)
	return events
}

func syntheticOutputItemAdded(deltaPayload []byte, sequenceNumber int) []byte {
	event := struct {
		Type        string `json:"type"`
		OutputIndex int    `json:"output_index"`
		Item        struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Status  string `json:"status"`
			Role    string `json:"role"`
			Content []any  `json:"content"`
		} `json:"item"`
		SequenceNumber int `json:"sequence_number"`
	}{
		Type:           "response.output_item.added",
		OutputIndex:    responseEventIndex(deltaPayload, "output_index"),
		SequenceNumber: sequenceNumber,
	}
	event.Item.ID = gjson.GetBytes(deltaPayload, "item_id").String()
	event.Item.Type = "message"
	event.Item.Status = "in_progress"
	event.Item.Role = "assistant"
	event.Item.Content = []any{}
	payload, _ := json.Marshal(event)
	return payload
}

func syntheticContentPartAdded(deltaPayload []byte, sequenceNumber int) []byte {
	event := struct {
		Type         string `json:"type"`
		ItemID       string `json:"item_id"`
		OutputIndex  int    `json:"output_index"`
		ContentIndex int    `json:"content_index"`
		Part         struct {
			Type        string `json:"type"`
			Annotations []any  `json:"annotations"`
			Logprobs    []any  `json:"logprobs"`
			Text        string `json:"text"`
		} `json:"part"`
		SequenceNumber int `json:"sequence_number"`
	}{
		Type:           "response.content_part.added",
		ItemID:         gjson.GetBytes(deltaPayload, "item_id").String(),
		OutputIndex:    responseEventIndex(deltaPayload, "output_index"),
		ContentIndex:   responseEventIndex(deltaPayload, "content_index"),
		SequenceNumber: sequenceNumber,
	}
	event.Part.Type = "output_text"
	event.Part.Annotations = []any{}
	event.Part.Logprobs = []any{}
	payload, _ := json.Marshal(event)
	return payload
}

func responseEventIndex(payload []byte, path string) int {
	result := gjson.GetBytes(payload, path)
	if !result.Exists() {
		return 0
	}
	return int(result.Int())
}
