package main

// 与 App 的 agent_protocol.dart 保持同一份 JSON 协议。

type Candidate struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
}

type PendingAction struct {
	ID        string         `json:"id"`
	ToolName  string         `json:"toolName"`
	Arguments map[string]any `json:"arguments"`
	Summary   string         `json:"summary"`
}

type Event struct {
	Type       string         `json:"type"`
	Text       string         `json:"text,omitempty"`
	Candidates []Candidate    `json:"candidates,omitempty"`
	Pending    *PendingAction `json:"pending,omitempty"`
}

const (
	eventMessage          = "message"
	eventCandidates       = "candidates"
	eventNeedConfirmation = "needConfirmation"
	eventError            = "error"
)
