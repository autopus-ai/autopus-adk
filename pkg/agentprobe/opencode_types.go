package agentprobe

import "time"

type OpenCodeOptions struct {
	Endpoint, Username, Password          string
	RuntimeVersion, ProviderID, ModelID   string
	Directory                             string
	Timeout, CleanupTimeout, PollInterval time.Duration
}

type OpenCodeTokens struct {
	Total     *int64 `json:"total,omitempty"`
	Input     *int64 `json:"input"`
	Output    *int64 `json:"output"`
	Reasoning *int64 `json:"reasoning"`
	Cache     struct {
		Read  *int64 `json:"read"`
		Write *int64 `json:"write"`
	} `json:"cache"`
}

type OpenCodeMessageUsage struct {
	SessionID       string          `json:"session_id"`
	MessageID       string          `json:"message_id"`
	ProviderID      string          `json:"provider_id,omitempty"`
	ModelID         string          `json:"model_id,omitempty"`
	Tokens          *OpenCodeTokens `json:"tokens"`
	Cost            *float64        `json:"cost"`
	Completed       bool            `json:"completed"`
	ErrorName       string          `json:"error_name,omitempty"`
	ErrorStatusCode *int            `json:"error_status_code,omitempty"`
}

type OpenCodeTransport struct {
	Protocol                string                 `json:"protocol"`
	Provenance              string                 `json:"provenance"`
	ObservedRuntimeVersion  string                 `json:"observed_runtime_version,omitempty"`
	SupervisorEmptyObserved bool                   `json:"supervisor_empty_observed"`
	Requests                int                    `json:"requests"`
	BytesReceived           int64                  `json:"bytes_received"`
	SupervisorID            string                 `json:"supervisor_id,omitempty"`
	ChildIDs                []string               `json:"child_ids"`
	CleanupConfirmed        bool                   `json:"cleanup_confirmed"`
	CaptureComplete         bool                   `json:"capture_complete"`
	Messages                []OpenCodeMessageUsage `json:"messages"`
	Failure                 string                 `json:"failure,omitempty"`
}

type openCodeSession struct {
	ID         string                                         `json:"id"`
	ParentID   string                                         `json:"parentID"`
	Title      string                                         `json:"title"`
	Permission []struct{ Permission, Pattern, Action string } `json:"permission"`
}

type openCodeMessage struct {
	Info struct {
		ID, SessionID, Role, ProviderID, ModelID string
		Time                                     struct {
			Completed *int64 `json:"completed"`
		} `json:"time"`
		Error *struct {
			Name string `json:"name"`
			Data struct {
				StatusCode *int `json:"statusCode"`
			} `json:"data"`
		} `json:"error"`
		Tokens *OpenCodeTokens `json:"tokens"`
		Cost   *float64        `json:"cost"`
	} `json:"info"`
	Parts []struct {
		Type, Text string
		SessionID  string `json:"sessionID"`
		MessageID  string `json:"messageID"`
	} `json:"parts"`
}
