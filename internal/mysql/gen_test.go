package mysql

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestArgName(t *testing.T) {
	tcase := [...]struct {
		input  string
		output string
	}{
		{
			input:  "get_users",
			output: "getUsers",
		},
		{
			input:  "get_users_by_id",
			output: "getUsersByID",
		},
		{
			input:  "get_all_",
			output: "getAll",
		},
	}

	for _, tc := range tcase {
		name := argName(tc.input)
		if diff := cmp.Diff(name, tc.output); diff != "" {
			t.Errorf(diff)
		}
	}
}

func TestEnumColumnValueName(t *testing.T) {
	cases := [...]struct {
		name, value string
		want        string
	}{
		struct {
			name  string
			value string
			want  string
		}{
			name:  "disabled",
			value: "true",
			want:  "DisabledTypeTrue",
		},
		struct {
			name  string
			value string
			want  string
		}{
			name:  "status",
			value: "on",
			want:  "StatusTypeOn",
		},
	}
	for _, tc := range cases {
		if diff := cmp.Diff(enumColumnValueName(tc.name, tc.value), tc.want); diff != "" {
			t.Errorf(diff)
		}
	}
}
