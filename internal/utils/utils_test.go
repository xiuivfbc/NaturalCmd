package utils

import "testing"

func TestValidateScriptOutput(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid command",
			input:   "git status",
			wantErr: false,
		},
		{
			name:    "valid cmd with args",
			input:   "@echo off",
			wantErr: false,
		},
		{
			name:    "chinese explanation",
			input:   "请执行 git status 查看当前状态",
			wantErr: true,
		},
		{
			name:    "markdown fence",
			input:   "```bash\ngit status\n```",
			wantErr: true,
		},
		{
			name:    "multi line output",
			input:   "git status\n然后继续",
			wantErr: true,
		},
		{
			name:    "shell launcher",
			input:   "powershell -NoProfile",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateScriptOutput(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}
