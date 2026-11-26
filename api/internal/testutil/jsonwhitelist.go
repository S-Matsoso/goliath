package testutil

import (
	"encoding/json"
	"testing"
)

type JSONWhitelistCase struct {
	Name    string
	Value   any
	Allowed []string
}

func AssertJSONWhitelists(t *testing.T, cases []JSONWhitelistCase) {
	t.Helper()
	for _, testCase := range cases {
		t.Run(testCase.Name, func(t *testing.T) {
			payload, err := json.Marshal(testCase.Value)
			if err != nil {
				t.Fatalf("marshal response: %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(payload, &fields); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			allowed := make(map[string]struct{}, len(testCase.Allowed))
			for _, name := range testCase.Allowed {
				allowed[name] = struct{}{}
			}
			for name := range fields {
				if _, ok := allowed[name]; !ok {
					t.Errorf("response includes field %q outside the DTO whitelist", name)
				}
			}
		})
	}
}
