package cli

import (
	"testing"
)

func TestParseProxyEnv(t *testing.T) {
	tests := []struct {
		name      string
		remoteEnv []string
		want      map[string]string
	}{
		{
			name:      "empty",
			remoteEnv: nil,
			want:      map[string]string{},
		},
		{
			name:      "single prefixed var",
			remoteEnv: []string{"--remote-env=FOO=bar"},
			want:      map[string]string{"FOO": "bar"},
		},
		{
			name:      "multiple prefixed vars",
			remoteEnv: []string{"--remote-env=HTTP_PROXY=http://host:1234", "--remote-env=HTTPS_PROXY=http://host:1234"},
			want:      map[string]string{"HTTP_PROXY": "http://host:1234", "HTTPS_PROXY": "http://host:1234"},
		},
		{
			name:      "unprefixed var falls through",
			remoteEnv: []string{"FOO=bar"},
			want:      map[string]string{"FOO": "bar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseProxyEnv(tt.remoteEnv)
			if len(got) != len(tt.want) {
				t.Fatalf("parseProxyEnv() = %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseProxyEnv()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

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
