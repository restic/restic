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
	"github.com/restic/restic/internal/repository"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/termstatus"
)

func testRunKeyListOtherIDs(t testing.TB, gopts GlobalOptions) []string {
	buf, err := withCaptureStdout(gopts, func(gopts GlobalOptions) error {
		return withTermStatus(gopts, func(ctx context.Context, term *termstatus.Terminal) error {
			return runKeyList(ctx, gopts, []string{}, term)
		})
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

	term, cancel := setupTermstatus()
	defer cancel()
	rtest.OK(t, runKeyAdd(context.TODO(), gopts, KeyAddOptions{}, []string{}, term))
}

func testRunKeyAddNewKeyUserHost(t testing.TB, gopts GlobalOptions) {
	testKeyNewPassword = "john's geheimnis"
	defer func() {
		testKeyNewPassword = ""
	}()

	t.Log("adding key for john@example.com")
	term, cancel := setupTermstatus()
	defer cancel()
	rtest.OK(t, runKeyAdd(context.TODO(), gopts, KeyAddOptions{
		Username: "john",
		Hostname: "example.com",
	}, []string{}, term))

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

	term, cancel := setupTermstatus()
	defer cancel()
	rtest.OK(t, runKeyPasswd(context.TODO(), gopts, KeyPasswdOptions{}, []string{}, term))
}

func testRunKeyRemove(t testing.TB, gopts GlobalOptions, IDs []string) {
	t.Logf("remove %d keys: %q\n", len(IDs), IDs)
	term, cancel := setupTermstatus()
	defer cancel()
	for _, id := range IDs {
		rtest.OK(t, runKeyRemove(context.TODO(), gopts, []string{id}, term))
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
	term, cancel := setupTermstatus()
	defer cancel()
	rtest.OK(t, runKeyList(context.TODO(), env.gopts, []string{}, term))
	testRunCheck(t, env.gopts)

	testRunKeyAddNewKeyUserHost(t, env.gopts)
}

func TestKeyAddInvalid(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testRunInit(t, env.gopts)

	term, cancel := setupTermstatus()
	defer cancel()
	err := runKeyAdd(context.TODO(), env.gopts, KeyAddOptions{
		NewPasswordFile:    "some-file",
		InsecureNoPassword: true,
	}, []string{}, term)
	rtest.Assert(t, strings.Contains(err.Error(), "only either"), "unexpected error message, got %q", err)

	pwfile := filepath.Join(t.TempDir(), "pwfile")
	rtest.OK(t, os.WriteFile(pwfile, []byte{}, 0o666))

	err = runKeyAdd(context.TODO(), env.gopts, KeyAddOptions{
		NewPasswordFile: pwfile,
	}, []string{}, term)
	rtest.Assert(t, strings.Contains(err.Error(), "an empty password is not allowed by default"), "unexpected error message, got %q", err)
}

func TestKeyAddEmpty(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	// must list keys more than once
	env.gopts.backendTestHook = nil
	defer cleanup()
	testRunInit(t, env.gopts)

	term, cancel := setupTermstatus()
	defer cancel()
	rtest.OK(t, runKeyAdd(context.TODO(), env.gopts, KeyAddOptions{
		InsecureNoPassword: true,
	}, []string{}, term))

	env.gopts.password = ""
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
	env.gopts.backendTestHook = func(r backend.Backend) (backend.Backend, error) {
		return &emptySaveBackend{r}, nil
	}

	testKeyNewPassword = "geheim2"
	defer func() {
		testKeyNewPassword = ""
	}()

	term, cancel := setupTermstatus()
	defer cancel()
	err := runKeyPasswd(context.TODO(), env.gopts, KeyPasswdOptions{}, []string{}, term)
	t.Log(err)
	rtest.Assert(t, err != nil, "expected passwd change to fail")

	err = runKeyAdd(context.TODO(), env.gopts, KeyAddOptions{}, []string{}, term)
	t.Log(err)
	rtest.Assert(t, err != nil, "expected key adding to fail")

	t.Logf("testing access with initial password %q\n", env.gopts.password)
	term2, cancel2 := setupTermstatus()
	defer cancel2()
	rtest.OK(t, runKeyList(context.TODO(), env.gopts, []string{}, term2))
	testRunCheck(t, env.gopts)
}

func TestKeyCommandInvalidArguments(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)
	env.gopts.backendTestHook = func(r backend.Backend) (backend.Backend, error) {
		return &emptySaveBackend{r}, nil
	}

	term, cancel := setupTermstatus()
	defer cancel()
	err := runKeyAdd(context.TODO(), env.gopts, KeyAddOptions{}, []string{"johndoe"}, term)
	t.Log(err)
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "no arguments"), "unexpected error for key add: %v", err)

	err = runKeyPasswd(context.TODO(), env.gopts, KeyPasswdOptions{}, []string{"johndoe"}, term)
	t.Log(err)
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "no arguments"), "unexpected error for key passwd: %v", err)

	term3, cancel3 := setupTermstatus()
	defer cancel3()
	err = runKeyList(context.TODO(), env.gopts, []string{"johndoe"}, term3)
	t.Log(err)
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "no arguments"), "unexpected error for key list: %v", err)

	err = runKeyRemove(context.TODO(), env.gopts, []string{}, term)
	t.Log(err)
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "one argument"), "unexpected error for key remove: %v", err)

	err = runKeyRemove(context.TODO(), env.gopts, []string{"john", "doe"}, term)
	t.Log(err)
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "one argument"), "unexpected error for key remove: %v", err)
}
