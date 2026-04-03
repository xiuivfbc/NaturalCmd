package completion

import "testing"

func TestExtractJSONObject(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain json",
			input: `{"selected_skills":["a"],"strategy":"x"}`,
			want:  `{"selected_skills":["a"],"strategy":"x"}`,
		},
		{
			name:  "markdown fenced json",
			input: "```json\n{\"selected_skills\":[\"a\"],\"strategy\":\"x\"}\n```",
			want:  `{"selected_skills":["a"],"strategy":"x"}`,
		},
		{
			name:  "with extra text",
			input: "result: {\"selected_skills\":[\"a\"],\"strategy\":\"x\"}",
			want:  `{"selected_skills":["a"],"strategy":"x"}`,
		},
		{
			name:  "invalid",
			input: "no json",
			want:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractJSONObject(tc.input); got != tc.want {
				t.Fatalf("extractJSONObject() = %q, want %q", got, tc.want)
			}
		})
	}
}
