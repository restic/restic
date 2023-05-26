package main

import (
	"bufio"
	"context"
	"regexp"
	"testing"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunKeyListOtherIDs(t testing.TB, gopts GlobalOptions) []string {
	buf, err := withCaptureStdout(func() error {
		return runKey(context.TODO(), gopts, []string{"list"})
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

func testRunKeyAddNewKey(t testing.TB, newPassword string, gopts GlobalOptions) {
	testKeyNewPassword = newPassword
	defer func() {
		testKeyNewPassword = ""
	}()

	rtest.OK(t, runKey(context.TODO(), gopts, []string{"add"}))
}

func testRunKeyAddNewKeyUserHost(t testing.TB, gopts GlobalOptions) {
	testKeyNewPassword = "john's geheimnis"
	defer func() {
		testKeyNewPassword = ""
		keyUsername = ""
		keyHostname = ""
	}()

	rtest.OK(t, cmdKey.Flags().Parse([]string{"--user=john", "--host=example.com"}))

	t.Log("adding key for john@example.com")
	rtest.OK(t, runKey(context.TODO(), gopts, []string{"add"}))

	repo, err := OpenRepository(context.TODO(), gopts)
	rtest.OK(t, err)
	key, err := repository.SearchKey(context.TODO(), repo, testKeyNewPassword, 2, "")
	rtest.OK(t, err)

	rtest.Equals(t, "john", key.Username)
	rtest.Equals(t, "example.com", key.Hostname)
}

func testRunKeyPasswd(t testing.TB, newPassword string, gopts GlobalOptions) {
	testKeyNewPassword = newPassword
	defer func() {
		testKeyNewPassword = ""
	}()

	rtest.OK(t, runKey(context.TODO(), gopts, []string{"passwd"}))
}

func testRunKeyRemove(t testing.TB, gopts GlobalOptions, IDs []string) {
	t.Logf("remove %d keys: %q\n", len(IDs), IDs)
	for _, id := range IDs {
		rtest.OK(t, runKey(context.TODO(), gopts, []string{"remove", id}))
	}
}

func TestKeyAddRemove(t *testing.T) {
	passwordList := []string{
		"OnnyiasyatvodsEvVodyawit",
		"raicneirvOjEfEigonOmLasOd",
	}

	env, cleanup := withTestEnvironment(t)
	// must list keys more than once
	env.gopts.backendTestHook = nil
	defer cleanup()

	testRunInit(t, env.gopts)

	testRunKeyPasswd(t, "geheim2", env.gopts)
	env.gopts.password = "geheim2"
	t.Logf("changed password to %q", env.gopts.password)

	for _, newPassword := range passwordList {
		testRunKeyAddNewKey(t, newPassword, env.gopts)
		t.Logf("added new password %q", newPassword)
		env.gopts.password = newPassword
		testRunKeyRemove(t, env.gopts, testRunKeyListOtherIDs(t, env.gopts))
	}

	env.gopts.password = passwordList[len(passwordList)-1]
	t.Logf("testing access with last password %q\n", env.gopts.password)
	rtest.OK(t, runKey(context.TODO(), env.gopts, []string{"list"}))
	testRunCheck(t, env.gopts)

	testRunKeyAddNewKeyUserHost(t, env.gopts)
}

type emptySaveBackend struct {
	restic.Backend
}

func (b *emptySaveBackend) Save(ctx context.Context, h restic.Handle, _ restic.RewindReader) error {
	return b.Backend.Save(ctx, h, restic.NewByteReader([]byte{}, nil))
}

func TestKeyProblems(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)
	env.gopts.backendTestHook = func(r restic.Backend) (restic.Backend, error) {
		return &emptySaveBackend{r}, nil
	}

	testKeyNewPassword = "geheim2"
	defer func() {
		testKeyNewPassword = ""
	}()

	err := runKey(context.TODO(), env.gopts, []string{"passwd"})
	t.Log(err)
	rtest.Assert(t, err != nil, "expected passwd change to fail")

	err = runKey(context.TODO(), env.gopts, []string{"add"})
	t.Log(err)
	rtest.Assert(t, err != nil, "expected key adding to fail")

	t.Logf("testing access with initial password %q\n", env.gopts.password)
	rtest.OK(t, runKey(context.TODO(), env.gopts, []string{"list"}))
	testRunCheck(t, env.gopts)
}
