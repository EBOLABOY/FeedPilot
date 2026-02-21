package auth

import "testing"

func TestExtractWebSessionFromHTML(t *testing.T) {
	tests := []struct {
		name        string
		html        string
		wantToken   string
		wantSession string
		wantBL      string
		wantErr     bool
	}{
		{
			name:        "extract token session and bl",
			html:        `{"SNlM0e":"abcdefghijklmnopqrstuv:1735689600000","FdrFJe":"1234567890123456789","cfb2h":"boq_labs-tailwind-frontend_20260120.01_p0"}`,
			wantToken:   "abcdefghijklmnopqrstuv:1735689600000",
			wantSession: "1234567890123456789",
			wantBL:      "boq_labs-tailwind-frontend_20260120.01_p0",
			wantErr:     false,
		},
		{
			name:        "extract token only when optional fields missing",
			html:        `{"SNlM0e":"abcdefghijklmnopqrstuvwxyz1234567890"}`,
			wantToken:   "abcdefghijklmnopqrstuvwxyz1234567890",
			wantSession: "",
			wantBL:      "",
			wantErr:     false,
		},
		{
			name:    "missing token should error",
			html:    `{"FdrFJe":"1234567890123456789"}`,
			wantErr: true,
		},
		{
			name:    "short token should error",
			html:    `{"SNlM0e":"too-short"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotToken, gotSession, gotBL, err := extractWebSessionFromHTML([]byte(tt.html))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotToken != tt.wantToken {
				t.Fatalf("token mismatch: got %q, want %q", gotToken, tt.wantToken)
			}
			if gotSession != tt.wantSession {
				t.Fatalf("session mismatch: got %q, want %q", gotSession, tt.wantSession)
			}
			if gotBL != tt.wantBL {
				t.Fatalf("bl mismatch: got %q, want %q", gotBL, tt.wantBL)
			}
		})
	}
}
