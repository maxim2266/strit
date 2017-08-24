/*
Copyright (c) 2017, Maxim Konakov
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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"
)

func TestSimpleStrings(t *testing.T) {
	ss := []string{"aaa", "bbb", "ccc"}
	fn := FromStrings(ss)
	i := 0

	if err := fn(func(line []byte) error {
		if string(line) != ss[i] {
			return fmt.Errorf("String mismatch at %d: %q instead of %q", i, string(line), ss[i])
		}

		i++
		return nil
	}); err != nil {
		t.Error(err)
	}
}

func TestSimpleMapFilter(t *testing.T) {
	if err := testABC("\n aaa \n    \n\n bbb\nccc  ", nil); err != nil {
		t.Error(err)
		return
	}

	if err := testABC("aaa  bbb  ccc ", bufio.ScanWords); err != nil {
		t.Error(err)
		return
	}
}

func testABC(src string, sf bufio.SplitFunc) error {
	iter := FromStringSF(sf, src).Map(bytes.TrimSpace).Filter(Not(Empty)).Map(bytes.ToUpper)

	// try multiple times to test the iterator reusability
	for i := 0; i < 3; i++ {
		s, err := iter.Join(", ")

		if err != nil {
			return err
		}

		const expect = "AAA, BBB, CCC"

		if s != expect {
			return fmt.Errorf("Unexpected string at %d: %q instead of %q", i, s, expect)
		}
	}

	return nil
}

func TestIterators(t *testing.T) {
	fileName, err := createFileOfInts("FileOfInts", 10)

	if err != nil {
		t.Error(err)
		return
	}

	defer os.Remove(fileName)

	fileIter := FromFile(fileName)

	tests := []struct {
		iter   Iter
		expect string
	}{
		{FromString("aaa \n bbb \n ccc ").Map(bytes.TrimSpace).Map(bytes.ToUpper),
			"AAA, BBB, CCC"},
		{FromString(" aaa \n bbb \n ccc ").Map(bytes.TrimSpace).GenMap(stopAtCcc),
			"aaa, bbb"},
		{FromString(" aaa \n bbb \n ccc ").Map(bytes.TrimSpace).GenMap(skipBbb),
			"aaa, ccc"},
		{FromString(" aaa \n bbb \n ccc ").Map(bytes.TrimSpace).TakeWhile(func(line []byte) bool {
			return bytes.ContainsAny(line, "ab")
		}),
			"aaa, bbb"},
		{FromString(" aaa \n bbb \n ccc ").Take(2).Map(bytes.TrimSpace),
			"aaa, bbb"},
		{FromString(" aaa \n bbb \n ccc ").Take(5).Map(bytes.TrimSpace),
			"aaa, bbb, ccc"},
		{FromString(" aaa \n bbb \n ccc ").Skip(2).Map(bytes.TrimSpace),
			"ccc"},
		{FromString(" aaa \n bbb \n ccc ").Skip(5).Map(bytes.TrimSpace),
			""},
		{FromString(" aaa \n bbb \n ccc ").Map(bytes.TrimSpace).SkipWhile(func(line []byte) bool {
			return bytes.ContainsAny(line, "ab")
		}),
			"ccc"},
		{Chain(FromString("aaa\nbbb\nccc"), FromString("xxx\nyyy\nzzz").Take(2)),
			"aaa, bbb, ccc, xxx, yyy"},
		{Chain(fileIter, fileIter.Take(2)), // test Chain() with early stop on file
			"0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1"},
		{FromStrings([]string{"aaa", "bbb", "###ccc", "ddd###"}).
			Filter(Not(StartsWith("###"))).
			Filter(Not(EndsWith("###"))).
			Filter(Not(StartsWith("@@@"))),
			"aaa, bbb"},
		{fileIter.Take(3),
			"0, 1, 2"},
		{FromStrings([]string{"aaa", "bbb", "ccc"}).Take(2), // test for EOF in FromStrings()
			"aaa, bbb"},
		// predicates
		{FromString("aaa\nbbb\nccc").Filter(StartsWith("aa").Or(StartsWith("bb"))),
			"aaa, bbb"},
		{FromString("aaa\nbbb\nccc").Filter(StartsWith("aa").And(EndsWith("aa"))),
			"aaa"},
		{FromString("aaa\naab\naac").Filter(StartsWith("aa").AndNot(EndsWith("c"))),
			"aaa, aab"},
		{FromString("aaa\nbbb\nccc").Filter(StartsWith("aa").OrNot(EndsWith("c"))),
			"aaa, bbb"},
		{FromString("aaa\nbbb\nccc").Filter(Pred(Empty).OrNot(EndsWith("c"))),
			"aaa, bbb"},
		{FromString(" aaa \n bbb \n ccc ").Map(bytes.TrimSpace).FirstNonEmpty(func(s []byte) []byte {
			if bytes.ContainsAny(s, "bc") {
				return bytes.ToUpper(s)
			}

			return nil
		}), "BBB"},
	}

	for i, test := range tests {
		res, err := test.iter.Join(", ")

		if err != nil {
			t.Error(err)
			return
		}

		if res != test.expect {
			t.Errorf("Unexpected result (%d): %q instead of %q", i, res, test.expect)
			return
		}
	}
}

func TestCatFileOfInts(t *testing.T) {
	name, err := createFileOfInts("FileOfInts", 200000)

	if err != nil {
		t.Error(err)
		return
	}

	defer os.Remove(name)

	var i int64

	if err = FromCommand(cat(), name)(func(line []byte) error {
		val, e := strconv.ParseInt(string(line), 10, 64)

		if e != nil {
			return e
		}

		if val != i {
			return fmt.Errorf("Value mismatch: %d instead of %d", val, i)
		}

		i++
		return nil

	}); err != nil {
		t.Error(err)
	}
}

func TestCatSearchFileOfInts(t *testing.T) {
	n := runtime.NumGoroutine() // the number of goroutines for later checks

	name, err := createFileOfInts("FileOfInts", 200000)

	if err != nil {
		t.Error(err)
		return
	}

	defer os.Remove(name)

	var i int64

	if err = FromCommand(cat(), name)(func(line []byte) error {
		val, e := strconv.ParseInt(string(line), 10, 64)

		if e != nil {
			return e
		}

		if val != i {
			return fmt.Errorf("Value mismatch: %d instead of %d", val, i)
		}

		if val == 100 { // early stop
			return io.EOF
		}

		i++
		return nil

	}); err != nil {
		t.Error(err)
		return
	}

	// wait for a short period of time to let other goroutines finish
	time.Sleep(100 * time.Millisecond)

	// see if the termination goroutine is still running
	if m := runtime.NumGoroutine(); m != n {
		t.Errorf("Number goroutines: before %d, after %d", n, m)
		return
	}
}

func TestCommandError(t *testing.T) {
	err := FromCommand(cat(), "nonexistent-file")(func(_ []byte) error {
		t.Error("Unexpected callback invocation")
		return io.EOF
	})

	if err == nil || err == io.EOF {
		t.Errorf("The command %q should have produced an error", cat())
		return
	}

	// println(err.Error())
}

func TestCommandTermination(t *testing.T) {
	if runtime.GOOS != "linux" {
		return
	}
	const msg = "Just an error"

	err := FromCommand("find", ".", "-type", "f")(func(s []byte) error {
		//println("Callback invoked with text: " + string(s))
		return errors.New(msg)
	})

	if err == nil {
		t.Error("Missing error")
		return
	}

	if s := err.Error(); s != msg {
		t.Errorf("Unexpected error message: %q instead of %q", s, msg)
		return
	}
}

func TestPipe(t *testing.T) {
	if runtime.GOOS != "linux" {
		return
	}

	const str = "aaa\nbbb\nccc"

	res, err := FromString(str).Pipe("cat").Pipe("cat").Pipe("cat").Join("\n")

	if err != nil {
		t.Error(err)
		return
	}

	if res != str {
		t.Errorf("Unexpected result: %q instead of %q", res, str)
		return
	}
}

func TestPipeTermination(t *testing.T) {
	if runtime.GOOS != "linux" {
		return
	}

	const msg = "Just an error"

	// termination at the end of the pipe
	err := FromString("aaa\nbbb\nccc").Pipe("cat")(func(s []byte) error {
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

	err = iter.Pipe("cat")(func(s []byte) error {
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

func TestNullTerminatedStrings(t *testing.T) {
	s, err := FromStringSF(ScanNullTerminatedStrings, "aaa\000bbb\000ccc\000").String()

	if err != nil {
		t.Error(err)
		return
	}

	if s != "aaabbbccc" {
		t.Errorf("Unexpected string: %q instead of \"aaabbbccc\"", s)
		return
	}
}

func TestWriteFile(t *testing.T) {
	name, err := tempFileName("xxx")

	if err != nil {
		t.Error(err)
		return
	}

	defer os.Remove(name)

	iter := FromString(" aaa \n bbb \n ccc ").Map(bytes.TrimSpace)

	// write file
	if _, err = iter.WriteToFile(name); err != nil {
		t.Error(err)
		return
	}

	// read file
	lines, err := FromFile(name).Strings() // just to test Strings() method as well

	if err != nil {
		t.Error(err)
		return
	}

	// compare
	expect := []string{"aaa", "bbb", "ccc"}

	if len(lines) != len(expect) {
		t.Errorf("Result size mismatch: %d instead of %d", len(lines), len(expect))
		return
	}

	for i := 0; i < len(lines); i++ {
		if lines[i] != expect[i] {
			t.Errorf("String mismatch at %d: %q instead of %q", i, lines[i], expect[i])
			return
		}
	}
}

func TestMerge(t *testing.T) {
	fileName, err := createFileOfInts("FileOfInts", 10)

	if err != nil {
		t.Error(err)
		return
	}

	defer os.Remove(fileName)

	tests := []struct {
		iter   Iter
		expect string
	}{
		{Merge(",", FromString("aaa\nbbb\nccc"), FromString("xxx\nyyy\nzzz"), FromString("111\n222\n333")),
			"aaa,xxx,111|bbb,yyy,222|ccc,zzz,333"},
		{Merge(",", FromString("aaa\nbbb\nccc"), FromString("xxx\nyyy\nzzz"), FromString("111\n222")),
			"aaa,xxx,111|bbb,yyy,222"},
		{Merge(",", FromString("aaa"), FromString("xxx\nyyy\nzzz"), FromString("111\n222\n333")),
			"aaa,xxx,111"},
		{Merge(",", FromString("aaa\nbbb\nccc")),
			"aaa|bbb|ccc"},
		{Merge(",", FromString("aaa\nbbb\nccc"), FromString("xxx\nyyy\nzzz").Take(2)),
			"aaa,xxx|bbb,yyy"},
		{Merge("", FromFile(fileName).Skip(5), FromFile(fileName).Take(5)),
			"50|61|72|83|94"},
	}

	for i, test := range tests {
		s, err := test.iter.Join("|")

		if err != nil {
			t.Error(i, err)
			return
		}

		if s != test.expect {
			t.Errorf("String mismatch [%d]: %q instead of %q", i, s, test.expect)
			return
		}
	}
}

func TestFromDir(t *testing.T) {
	expect := map[string]int{
		"strit.go":      0,
		"strit_test.go": 0,
		"LICENSE":       0,
		"README.md":     0,
	}

	if err := FromDir(".", nil)(func(line []byte) error {
		delete(expect, string(line))
		//		t.Log("#", string(line))
		return nil
	}); err != nil {
		t.Error(err)
		return
	}

	if len(expect) > 0 {
		var names []string

		for name := range expect {
			names = append(names, name)
		}

		t.Errorf("Skipped entries: %s", strings.Join(names, ", "))
		return
	}
}

func TestFromDirWalk(t *testing.T) {
	// temporary directory
	dir, err := ioutil.TempDir("", "DirWalk")

	if err != nil {
		t.Error(err)
		return
	}

	defer os.RemoveAll(dir)

	// a few files
	fileNames := [...]string{"aaa", "bbb", "ccc"}

	for _, file := range fileNames[:] {
		if err = makeFile(dir, file); err != nil {
			t.Error(err)
			return
		}
	}

	// sub-directory
	const subDir = "subdir"

	if err = os.Mkdir(filepath.Join(dir, subDir), 0777); err != nil {
		t.Error(err)
		return
	}

	// a few files in the sub-directory
	for _, file := range fileNames[:] {
		if err = makeFile(dir, subDir, file); err != nil {
			t.Error(err)
			return
		}
	}

	// tests
	tests := []struct {
		wf     filepath.WalkFunc
		expect map[string]int
	}{
		{
			func(_ string, _ os.FileInfo, _ error) error { return nil }, // accept everything
			map[string]int{
				dir: 0,
				filepath.Join(dir, "aaa"):         0,
				filepath.Join(dir, "bbb"):         0,
				filepath.Join(dir, "ccc"):         0,
				filepath.Join(dir, subDir):        0,
				filepath.Join(dir, subDir, "aaa"): 0,
				filepath.Join(dir, subDir, "bbb"): 0,
				filepath.Join(dir, subDir, "ccc"): 0,
			},
		},
		{
			func(_ string, info os.FileInfo, _ error) error {
				if info.Name() == subDir {
					return filepath.SkipDir
				}

				return nil
			},
			map[string]int{
				dir: 0,
				filepath.Join(dir, "aaa"): 0,
				filepath.Join(dir, "bbb"): 0,
				filepath.Join(dir, "ccc"): 0,
			},
		},
		{
			func(_ string, info os.FileInfo, _ error) error {
				if info.Name() == "bbb" {
					return filepath.SkipDir
				}

				return nil
			},
			map[string]int{
				dir: 0,
				filepath.Join(dir, "aaa"): 0,
			},
		},
	}

	for i, test := range tests {
		if err = FromDirWalk(dir, test.wf)(func(line []byte) error {
			//		t.Log(string(line))
			path := string(line)
			_, ok := test.expect[path]

			if !ok {
				return fmt.Errorf("Unexpected path (%d): %q", i, path)
			}

			delete(test.expect, path)
			return nil
		}); err != nil {
			t.Error(err)
			return
		}

		if len(test.expect) > 0 {
			var remaining []string

			for path := range test.expect {
				remaining = append(remaining, path)
			}

			t.Errorf("Remaining items (%d): %s", i, strings.Join(remaining, ", "))
			return
		}
	}
}

func TestStringFromBytes(t *testing.T) {
	res, err := FromBytes([]byte("aaa\nbbb\nccc")).String()

	if err != nil {
		t.Error(err)
		return
	}

	if res != "aaabbbccc" {
		t.Errorf("Unexpected result: %q", res)
		return
	}
}

func TestBytesFromBytes(t *testing.T) {
	res, err := FromBytes([]byte("aaa\nbbb\nccc")).Bytes()

	if err != nil {
		t.Error(err)
		return
	}

	if bytes.Compare(res, []byte("aaabbbccc")) != 0 {
		t.Errorf("Unexpected result: %q", string(res))
		return
	}
}

func TestJoinBytes(t *testing.T) {
	res, err := FromString("aaa\nbbb\nccc").JoinBytes(" ")

	if err != nil {
		t.Error(err)
		return
	}

	if bytes.Compare(res, []byte("aaa bbb ccc")) != 0 {
		t.Errorf("Unexpected result: %q", string(res))
		return
	}
}

// parser test
type dataItem struct {
	A, B, C int
}

func TestParser(t *testing.T) {
	const input = `
	A 1
	B 2
	C 3
	
	A 4
	B 5
	C 6
	
	A 7
	B 8
	C 9`

	i := 1

	fn := func(item *dataItem) error {
		if item.A != i || item.B != i+1 || item.C != i+2 {
			return fmt.Errorf("Unexpected value: A = %d, B = %d, C = %d", item.A, item.B, item.C)
		}

		i += 3
		return nil
	}

	err := FromString(input).Map(bytes.TrimSpace).Filter(Not(Empty)).Parse(&dataItemParser{fn: fn})

	if err != nil {
		t.Error(err)
		return
	}

	if i != 10 {
		t.Errorf("Unexpected value of counter: %d instead of 10", i)
		return
	}
}

type dataItemParser struct {
	fn   func(*dataItem) error
	item *dataItem
}

func (p *dataItemParser) Done(err error) error {
	return err
}

func (p *dataItemParser) Enter(s []byte) (ParserFunc, error) {
	var err error

	p.item = new(dataItem)
	p.item.A, err = getField("A", s)
	return p.getB, err
}

func (p *dataItemParser) getB(s []byte) (ParserFunc, error) {
	var err error

	p.item.B, err = getField("B", s)
	return p.getC, err
}

func (p *dataItemParser) getC(s []byte) (ParserFunc, error) {
	var err error

	if p.item.C, err = getField("C", s); err != nil {
		return nil, err
	}

	return p.Enter, p.fn(p.item)
}

func getField(name string, s []byte) (int, error) {
	fields := bytes.FieldsFunc(s, unicode.IsSpace)

	if len(fields) != 2 {
		return 0, errors.New("Invalid number of fields: " + string(s))
	}

	k, v := string(fields[0]), string(fields[1])

	if k != name {
		return 0, errors.New("Invalid field name: " + k)
	}

	return strconv.Atoi(v)
}

// benchmarks
var benchData string
var benchDataSum int

func init() {
	var buff bytes.Buffer

	for i := 0; i < 1000; i++ {
		val := rand.Int()

		benchDataSum += val
		buff.WriteString("  " + strconv.Itoa(val) + "  \n")

		switch rand.Int() % 4 {
		case 0:
			buff.WriteString("  zzz  \n")
		case 1:
			buff.WriteString("   \n")
		}
	}

	benchData = buff.String()
}

func BenchmarkScanner(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var sum int

		src := bufio.NewScanner(bytes.NewBufferString(benchData))

		for src.Scan() {
			line := bytes.TrimSpace(src.Bytes())

			if len(line) > 0 {
				if val, err := strconv.Atoi(string(line)); err == nil {
					sum += val
				}
			}
		}

		if err := src.Err(); err != nil {
			b.Error(err)
			return
		}

		if sum != benchDataSum {
			b.Errorf("Invalid sum: %d instead of %d", sum, benchDataSum)
			return
		}
	}
}

func BenchmarkFromString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var sum int

		if err := FromString(benchData).Map(bytes.TrimSpace).Filter(Not(Empty))(func(line []byte) error {
			if val, e := strconv.Atoi(string(line)); e == nil {
				sum += val
			}

			return nil
		}); err != nil {
			b.Error(err)
			return
		}

		if sum != benchDataSum {
			b.Errorf("Invalid sum: %d instead of %d", sum, benchDataSum)
			return
		}
	}
}

// helper functions
func createFileOfInts(prefix string, N int64) (string, error) {
	return withTempFileWriter(prefix, func(file io.Writer) (err error) {
		for i := int64(0); i < N && err == nil; i++ {
			_, err = fmt.Fprintf(file, "%d\n", i)
		}

		return
	})
}

func withTempFileWriter(prefix string, fn func(io.Writer) error) (fname string, err error) {
	var file *os.File

	if file, err = ioutil.TempFile("", prefix); err != nil {
		return
	}

	fname = file.Name()

	defer func() {
		if e := file.Close(); e != nil && err == nil {
			err = e
		}

		if err != nil {
			os.Remove(fname)
		}
	}()

	err = fn(file)
	return
}

func tempFileName(prefix string) (name string, err error) {
	var file *os.File

	if file, err = ioutil.TempFile("", prefix); err != nil {
		return
	}

	defer file.Close()

	name = file.Name()
	return
}

func cat() string {
	switch runtime.GOOS {
	case "windows":
		return "type"
	default:
		return "cat"
	}
}

func stopAtCcc(line []byte) ([]byte, error) {
	if bytes.Compare(line, []byte("ccc")) == 0 {
		return nil, io.EOF
	}

	return line, nil
}

func skipBbb(line []byte) ([]byte, error) {
	if bytes.Compare(line, []byte("bbb")) == 0 {
		return nil, ErrSkip
	}

	return line, nil
}

func makeFile(elem ...string) error {
	return ioutil.WriteFile(filepath.Join(elem...), []byte(elem[len(elem)-1]), 0666)
}
