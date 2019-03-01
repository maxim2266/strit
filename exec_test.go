//+build linux darwin

/*
Copyright (c) 2017,2018,2019 Maxim Konakov
All rights reserved.

Redistribution and use in source and binary forms, with or without modification,
are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice,
   this list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.
3. Neither the name of the copyright holder nor the names of its contributors
   may be used to endorse or promote products derived from this software without
   specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED.
IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT,
INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING,
BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY
OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING
NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE,
EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

package strit

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"testing"
)

func TestLimitedWriter(t *testing.T) {
	type testCase struct{ a, b []byte }

	cases := []testCase{
		{[]byte("a"), []byte("a")},
		{[]byte("ab"), []byte("ab")},
		{[]byte("abcdef"), []byte("abcde")},
		{[]byte("abcdefghijklmopqrtuvwxyz"), []byte("abcde")},
	}

	for i, c := range cases {
		var s limitedWriter

		s.limit = 5
		n, err := s.Write(c.a)

		switch {
		case n != len(c.a):
			t.Errorf("(%d) Unexpected write length: %d insted of %d", i, n, len(c.a))
			return
		case err != nil:
			t.Errorf("(%d) Unexpected error: %s", i, err)
			return
		case bytes.Compare(s.buff, c.b) != 0:
			t.Errorf("(%d) Unexpected result: %q instead of %q", i, string(s.buff), string(c.b))
			return
		}
	}
}

func TestLimitedWriter2(t *testing.T) {
	type testCase struct{ a, b, exp []byte }

	cases := []testCase{
		{[]byte("a"), []byte("b"), []byte("ab")},
		{[]byte("abcdef"), nil, []byte("abcde")},
		{[]byte("abcd"), []byte("e"), []byte("abcde")},
		{[]byte("abcd"), []byte("efg"), []byte("abcde")},
		{[]byte("abcd"), []byte("efghijklmopqrtuvwxyz"), []byte("abcde")},
		{[]byte("abcde"), []byte("fghijklmopqrtuvwxyz"), []byte("abcde")},
		{[]byte("abcdef"), []byte("ghijklmopqrtuvwxyz"), []byte("abcde")},
	}

	for i, c := range cases {
		var s limitedWriter

		s.limit = 5

		s.Write(c.a)
		s.Write(c.b)

		switch {
		case len(s.buff) != len(c.exp):
			t.Errorf("(%d) Unexpected write length: %d insted of %d", i, len(s.buff), len(c.exp))
			return
		case bytes.Compare(s.buff, c.exp) != 0:
			t.Errorf("(%d) Unexpected result: %q instead of %q", i, string(s.buff), string(c.exp))
			return
		}
	}
}

func TestEcho(t *testing.T) {
	type testCase struct {
		in  string
		exp []string
	}

	cases := []testCase{
		{`echo "ZZZ"`, []string{"ZZZ"}},
		{`echo "ZZZ" ; echo "zzz"`, []string{"ZZZ", "zzz"}},
	}

	for i, c := range cases {
		var buff []string

		ret, err := invokeCmd(exec.Command("sh", "-c", c.in), func(s []byte) error {
			buff = append(buff, string(s))
			return nil
		})

		switch {
		case err != nil:
			t.Errorf("(%d) Unexpected error: [%d] %s", i, ret, err)
			return
		case ret != 0:
			t.Errorf("(%d) Unexpected exit code %d without an error", i, ret)
			return
		case len(buff) != len(c.exp):
			t.Errorf("(%d) Unexpected number of lines: %d instead of %d", i, len(buff), len(c.exp))
			return
		}

		for i := 0; i < len(buff); i++ {
			if buff[i] != c.exp[i] {
				t.Errorf("(%d) Unexpected line: %q instead of %q", i, buff[i], c.exp[i])
				return
			}
		}
	}
}

func TestErrors(t *testing.T) {
	type testCase struct {
		cmd   *exec.Cmd
		check checkFunc
	}

	cases := []testCase{
		{
			exec.Command("find", ".", "-type", "f", "-name", "*.go"),
			checkOutput("./exec_test.go", "./strit.go", "./strit_test.go"),
		},
		{
			exec.Command("findZZZ", ".", "-type", "f", "-name", "*.go"),
			hasError(t),
		},
		{
			exec.Command("find", "./this-does-not-exist", "-type", "f", "-name", "*.go"),
			hasError(t),
		},
	}

	for i, c := range cases {
		var buff []string

		ret, err := invokeCmd(c.cmd, func(s []byte) error {
			buff = append(buff, string(s))
			return nil
		})

		if msg := c.check(ret, err, buff); len(msg) > 0 {
			t.Errorf("(%d) %s", i, msg)
			return
		}
	}
}

func TestBreak(t *testing.T) {
	var count int
	var res []string

	ret, err := invokeCmd(exec.Command("sh", "-c", `echo "AAA" ; echo "BBB" ; echo "CCC"`), func(s []byte) error {
		if count++; count > 2 {
			return io.EOF
		}

		res = append(res, string(s))
		return nil
	})

	switch {
	case err != nil:
		t.Errorf("Unexpected error: [%d] %s", ret, err)
		return
	case ret != 0:
		t.Errorf("Unexpected exit code %d without an error", ret)
		return
	case len(res) != 2:
		t.Errorf("Unexpected number of lines: %d instead of 3", len(res))
		return
	}

	for i, s := range []string{"AAA", "BBB"} {
		if s != res[i] {
			t.Errorf("(%d) unexpected string: %q instead of %q", i, res[i], s)
			return
		}
	}
}

func TestPipeTermination(t *testing.T) {
	const msg = "Just an error"

	// termination at the end of the pipe
	err := FromString("aaa\nbbb\nccc").Pipe(exec.Command("cat"))(func(s []byte) error {
		if bytes.Compare(s, []byte("aaa")) != 0 {
			return fmt.Errorf("Invalid string in callback: %q", string(s))
		}

		return errors.New(msg)
	})

	if err == nil {
		t.Error("Missing error")
		return
	}

	if err.Error() != msg {
		t.Errorf("Unexpected error: %q instead of %q", err.Error(), msg)
		return
	}

	// termination before pipe
	iter := Iter(func(fn Func) error {
		if err := fn([]byte("aaa")); err != nil {
			return err
		}

		return errors.New(msg)
	})

	err = iter.Pipe(exec.Command("cat"))(func(s []byte) error {
		if bytes.Compare(s, []byte("aaa")) != 0 {
			return fmt.Errorf("Invalid string in callback: %q", string(s))
		}

		return nil
	})

	if err == nil {
		t.Error("Missing error")
		return
	}

	if err.Error() != msg {
		t.Errorf("Unexpected error: %q instead of %q", err.Error(), msg)
		return
	}
}

// helpers
type checkFunc = func(int, error, []string) string

func checkOutput(exp ...string) checkFunc {
	return func(ret int, err error, out []string) string {
		switch {
		case err != nil:
			return fmt.Sprintf("Unexpected error message: %q", err)
		case ret != 0:
			return fmt.Sprintf("Unexpected exit code %d without error", ret)
		case len(out) != len(exp):
			return fmt.Sprintf("Unexpected number of lines: %d instead of %d", len(out), len(exp))
		}

		sort.Strings(out)

		for i := 0; i < len(out); i++ {
			if out[i] != exp[i] {
				return fmt.Sprintf("Unexpected line: %q instead of %q", out[i], exp[i])
			}
		}

		return ""
	}
}

func hasError(t *testing.T) checkFunc {
	return func(ret int, err error, out []string) string {
		switch {
		case ret == 0:
			return "Unexpected exit code: 0"
		case err == nil:
			return "Missing error message"
		}

		t.Logf("exit code: %d, error message: %q", ret, err)
		return ""
	}
}

func invokeCmd(cmd *exec.Cmd, fn Func) (ret int, err error) {
	if err = FromCommand(cmd)(fn); err != nil {
		if e, ok := err.(*ExitError); ok {
			ret = e.ExitCode
		} else {
			ret = -1
		}
	}

	return
}
