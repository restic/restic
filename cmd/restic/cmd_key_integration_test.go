package main

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
)

func testRunKeyListOtherIDs(t testing.TB, gopts global.Options) []string {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyList(ctx, gopts, []string{}, gopts.Term)
	})
	rtest.OK(t, err)

	scanner := bufio.NewScanner(buf)
	exp := regexp.MustCompile(`^ ([a-f0-9]+) `)

	IDs := []string{}
	for scanner.Scan() {
		if id := exp.FindStringSubmatch(scanner.Text()); id != nil {
			IDs = append(IDs, id[1])
		}
	}

	return IDs
}

func testRunKeyAddNewKey(t testing.TB, newPassword string, gopts global.Options) {
	testKeyNewPassword = newPassword
	defer func() {
		testKeyNewPassword = ""
	}()

	err := withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyAdd(ctx, gopts, KeyAddOptions{}, []string{}, gopts.Term)
	})
	rtest.OK(t, err)
}

func testRunKeyAddNewKeyUserHost(t testing.TB, gopts global.Options) {
	testKeyNewPassword = "john's geheimnis"
	defer func() {
		testKeyNewPassword = ""
	}()

	t.Log("adding key for john@example.com")
	err := withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyAdd(ctx, gopts, KeyAddOptions{
			Username: "john",
			Hostname: "example.com",
		}, []string{}, gopts.Term)
	})
	rtest.OK(t, err)

	_ = withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		repo, err := global.OpenRepository(ctx, gopts, &progress.NoopPrinter{})
		rtest.OK(t, err)
		key, err := repository.SearchKey(ctx, repo, testKeyNewPassword, 2, "")
		rtest.OK(t, err)

		rtest.Equals(t, "john", key.Username)
		rtest.Equals(t, "example.com", key.Hostname)
		return nil
	})
}

func testRunKeyPasswd(t testing.TB, newPassword string, gopts global.Options) {
	testKeyNewPassword = newPassword
	defer func() {
		testKeyNewPassword = ""
	}()

	err := withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyPasswd(ctx, gopts, KeyPasswdOptions{}, []string{}, gopts.Term)
	})
	rtest.OK(t, err)
}

func testRunKeyRemove(t testing.TB, gopts global.Options, IDs []string) {
	t.Logf("remove %d keys: %q\n", len(IDs), IDs)
	for _, id := range IDs {
		err := withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
			return runKeyRemove(ctx, gopts, []string{id}, gopts.Term)
		})
		rtest.OK(t, err)
	}
}

func TestKeyAddRemove(t *testing.T) {
	passwordList := []string{
		"OnnyiasyatvodsEvVodyawit",
		"raicneirvOjEfEigonOmLasOd",
	}

	env, cleanup := withTestEnvironment(t)
	// must list keys more than once
	env.gopts.BackendTestHook = nil
	defer cleanup()

	testRunInit(t, env.gopts)

	testRunKeyPasswd(t, "geheim2", env.gopts)
	env.gopts.Password = "geheim2"
	t.Logf("changed password to %q", env.gopts.Password)

	for _, newPassword := range passwordList {
		testRunKeyAddNewKey(t, newPassword, env.gopts)
		t.Logf("added new password %q", newPassword)
		env.gopts.Password = newPassword
		testRunKeyRemove(t, env.gopts, testRunKeyListOtherIDs(t, env.gopts))
	}

	env.gopts.Password = passwordList[len(passwordList)-1]
	t.Logf("testing access with last password %q\n", env.gopts.Password)
	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyList(ctx, gopts, []string{}, gopts.Term)
	})
	rtest.OK(t, err)
	testRunCheck(t, env.gopts)

	testRunKeyAddNewKeyUserHost(t, env.gopts)
}

func TestKeyAddInvalid(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testRunInit(t, env.gopts)

	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyAdd(ctx, gopts, KeyAddOptions{
			NewPasswordFile:    "some-file",
			InsecureNoPassword: true,
		}, []string{}, gopts.Term)
	})
	rtest.Assert(t, strings.Contains(err.Error(), "only either"), "unexpected error message, got %q", err)

	pwfile := filepath.Join(t.TempDir(), "pwfile")
	rtest.OK(t, os.WriteFile(pwfile, []byte{}, 0o666))

	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyAdd(ctx, gopts, KeyAddOptions{
			NewPasswordFile: pwfile,
		}, []string{}, gopts.Term)
	})
	rtest.Assert(t, strings.Contains(err.Error(), "an empty password is not allowed by default"), "unexpected error message, got %q", err)
}

func TestKeyAddEmpty(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	// must list keys more than once
	env.gopts.BackendTestHook = nil
	defer cleanup()
	testRunInit(t, env.gopts)

	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyAdd(ctx, gopts, KeyAddOptions{
			InsecureNoPassword: true,
		}, []string{}, gopts.Term)
	})
	rtest.OK(t, err)

	env.gopts.Password = ""
	env.gopts.InsecureNoPassword = true

	testRunCheck(t, env.gopts)
}

type emptySaveBackend struct {
	backend.Backend
}

func (b *emptySaveBackend) Save(ctx context.Context, h backend.Handle, _ backend.RewindReader) error {
	return b.Backend.Save(ctx, h, backend.NewByteReader([]byte{}, nil))
}

func TestKeyProblems(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)
	env.gopts.BackendTestHook = func(r backend.Backend) (backend.Backend, error) {
		return &emptySaveBackend{r}, nil
	}

	testKeyNewPassword = "geheim2"
	defer func() {
		testKeyNewPassword = ""
	}()

	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyPasswd(ctx, gopts, KeyPasswdOptions{}, []string{}, gopts.Term)
	})
	t.Log(err)
	rtest.Assert(t, err != nil, "expected passwd change to fail")

	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyAdd(ctx, gopts, KeyAddOptions{}, []string{}, gopts.Term)
	})
	t.Log(err)
	rtest.Assert(t, err != nil, "expected key adding to fail")

	t.Logf("testing access with initial password %q\n", env.gopts.Password)
	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyList(ctx, gopts, []string{}, gopts.Term)
	})
	rtest.OK(t, err)
	testRunCheck(t, env.gopts)
}

func TestKeyCommandInvalidArguments(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)
	env.gopts.BackendTestHook = func(r backend.Backend) (backend.Backend, error) {
		return &emptySaveBackend{r}, nil
	}

	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyAdd(ctx, gopts, KeyAddOptions{}, []string{"johndoe"}, gopts.Term)
	})
	t.Log(err)
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "no arguments"), "unexpected error for key add: %v", err)

	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyPasswd(ctx, gopts, KeyPasswdOptions{}, []string{"johndoe"}, gopts.Term)
	})
	t.Log(err)
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "no arguments"), "unexpected error for key passwd: %v", err)

	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyList(ctx, gopts, []string{"johndoe"}, gopts.Term)
	})
	t.Log(err)
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "no arguments"), "unexpected error for key list: %v", err)

	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyRemove(ctx, gopts, []string{}, gopts.Term)
	})
	t.Log(err)
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "one argument"), "unexpected error for key remove: %v", err)

	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runKeyRemove(ctx, gopts, []string{"john", "doe"}, gopts.Term)
	})
	t.Log(err)
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "one argument"), "unexpected error for key remove: %v", err)
}
