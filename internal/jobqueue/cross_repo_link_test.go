package jobqueue

import "testing"

func TestRepositoryIdentities(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{
			in:   "https://git.apps.hexmos.com/framework-examples/typescript/nestjs/blive-microservice-2-consumer",
			want: []string{"https://git.apps.hexmos.com/framework-examples/typescript/nestjs/blive-microservice-2-consumer", "framework-examples/typescript/nestjs/blive-microservice-2-consumer"},
		},
		{
			in:   "framework-examples/typescript/nestjs/blive-microservice-2-consumer",
			want: []string{"framework-examples/typescript/nestjs/blive-microservice-2-consumer"},
		},
		{
			in:   "https://git.example.com/group/sub/repo.git",
			want: []string{"https://git.example.com/group/sub/repo.git", "group/sub/repo"},
		},
		{
			in:   "https://git.example.com/group/repo/-/merge_requests/7",
			want: []string{"https://git.example.com/group/repo/-/merge_requests/7", "group/repo"},
		},
	}
	for _, tc := range cases {
		got := repositoryIdentities(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("repositoryIdentities(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("repositoryIdentities(%q)[%d] = %q, want %q (full: %v)", tc.in, i, got[i], tc.want[i], got)
			}
		}
	}
}
