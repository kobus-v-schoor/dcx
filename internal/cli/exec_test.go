package cli

import (
	"testing"
)

func TestMergeExecEnv(t *testing.T) {
	tests := []struct {
		name      string
		remoteEnv map[string]string
		proxyEnv  map[string]string
		want      []string
	}{
		{
			name:      "empty both",
			remoteEnv: nil,
			proxyEnv:  nil,
			want:      []string{},
		},
		{
			name:      "only remoteEnv",
			remoteEnv: map[string]string{"A": "1", "B": "2"},
			proxyEnv:  nil,
			want:      []string{"A=1", "B=2"},
		},
		{
			name:      "only proxyEnv",
			remoteEnv: nil,
			proxyEnv:  map[string]string{"C": "3"},
			want:      []string{"C=3"},
		},
		{
			name:      "merged no conflict",
			remoteEnv: map[string]string{"A": "1"},
			proxyEnv:  map[string]string{"B": "2"},
			want:      []string{"A=1", "B=2"},
		},
		{
			name:      "proxy wins conflict",
			remoteEnv: map[string]string{"A": "1"},
			proxyEnv:  map[string]string{"A": "2"},
			want:      []string{"A=2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeExecEnv(tt.remoteEnv, tt.proxyEnv)
			if len(got) != len(tt.want) {
				t.Fatalf("mergeExecEnv() = %v, want %v", got, tt.want)
			}
			for i, want := range tt.want {
				if got[i] != want {
					t.Errorf("mergeExecEnv()[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}
