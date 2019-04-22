package main

import (
	"reflect"
	"testing"
)

func Test_acl_decode(t *testing.T) {
	type args struct {
		xattr []byte
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "decode string",
			args: args{
				xattr: []byte{2, 0, 0, 0, 1, 0, 6, 0, 255, 255, 255, 255, 2, 0, 7, 0, 0, 0, 0, 0, 2, 0, 7, 0, 254, 255, 0, 0, 4, 0, 7, 0, 255, 255, 255, 255, 16, 0, 7, 0, 255, 255, 255, 255, 32, 0, 4, 0, 255, 255, 255, 255},
			},
			want: "user::rw-\nuser:0:rwx\nuser:65534:rwx\ngroup::rwx\nmask::rwx\nother::r--\n",
		},
		{
			name: "decode fail",
			args: args{
				xattr: []byte("abctest"),
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &acl{}
			a.decode(tt.args.xattr)
			if tt.want != a.String() {
				t.Errorf("acl.decode() = %v, want: %v", a.String(), tt.want)
			}
		})
	}
}

func Test_acl_encode(t *testing.T) {
	tests := []struct {
		name string
		want []byte
		args []aclElement
	}{
		{
			name: "encode values",
			want: []byte{2, 0, 0, 0, 1, 0, 6, 0, 255, 255, 255, 255, 2, 0, 7, 0, 0, 0, 0, 0, 2, 0, 7, 0, 254, 255, 0, 0, 4, 0, 7, 0, 255, 255, 255, 255, 16, 0, 7, 0, 255, 255, 255, 255, 32, 0, 4, 0, 255, 255, 255, 255},
			args: []aclElement{
				{
					aclSID: 8589934591,
					Perm:   6,
				},
				{
					aclSID: 8589934592,
					Perm:   7,
				},
				{
					aclSID: 8590000126,
					Perm:   7,
				},
				{
					aclSID: 21474836479,
					Perm:   7,
				},
				{
					aclSID: 73014444031,
					Perm:   7,
				},
				{
					aclSID: 141733920767,
					Perm:   4,
				},
			},
		},
		{
			name: "encode fail",
			want: []byte{2, 0, 0, 0},
			args: []aclElement{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &acl{
				Version: 2,
				List:    tt.args,
			}
			if got := a.encode(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("acl.encode() = %v, want %v", got, tt.want)
			}
		})
	}
}
