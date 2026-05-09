package containerengine

import (
	"reflect"
	"testing"
)

func TestSeccompSecurityOpts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    string
		want    []string
		wantErr bool
	}{
		{name: "empty uses docker default", mode: "", want: nil},
		{name: "default uses docker default", mode: " default ", want: nil},
		{name: "docker default alias", mode: "docker-default", want: nil},
		{name: "unconfined", mode: " UNCONFINED ", want: []string{"seccomp=unconfined"}},
		{name: "unsupported", mode: "strict", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := SeccompSecurityOpts(tc.mode)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("SeccompSecurityOpts() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("SeccompSecurityOpts() error = %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SeccompSecurityOpts() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
