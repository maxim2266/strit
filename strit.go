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

/*
Package strit introduces a string iterator of "push" type, as well as a number of
iterator constructors and wrappers.

Typically, iteration over some source of data involves creating an object of some
reader type, then repeatedly calling a method on the object to get the data elements,
and finally calling some method to release the resources allocated inside the reader object.
And in the languages without exceptions (like Go) there must be an error check on each step.
In other words, there is an interface and some protocol all the clients must follow,
which usually results in certain amount of boilerplate code to write.

The above example describes the so called "pull" model of iteration where the client has to call a method
on the reader to "pull" the next data element out of the reader object. Alternative to that
is the "push" model where client gives the iterator a callback function to be invoked once
per each element of input data.

Another aspect of the iteration model is about dealing with errors. In the absence of exceptions,
like in Go language, the "pull" model implies that the result of each step of the
iteration is either a data item, or an error. The same is true for pure "push" model where
each invocation of the callback has to receive two parameters: a data element and an error. Or there
maybe two callbacks, one for data and another for errors, but this is conceptually the same.

This library is an experiment in combining both "push" and "pull" models: it utilises "push"
model for data elements, and "pull" model for error propagation. The data elements here are strings only,
as currently the Go language does not offer any generic programming mechanism. The iterator type itself is
a function that invokes the supplied callback once per each input string, and it returns either an error or nil.
This arrangement reduces the amount of boilerplate code, at least in the most
common scenarios, plus it allows for easy iterator combination, especially useful where dynamic creation
of string processing pipelines is required.

The string iterator type used throughout this library is simply a function of the type func(Func) error,
where Func is the type of the callback, func([]byte) error. Strings are represented as byte slices
for performance reasons, to allow for in-place modifications where possible, like in bytes.TrimSpace() function.
Iteration is simply an invocation of the iterator function with a callback to receive strings. The
iterator function returns whatever error may have occurred during the iteration. The callback
function itself may also return an error which will stop the iteration and will be returned from
the iterator function. The special error value of io.EOF is treated as a request to stop the iteration and
will not be propagated up to the caller.

The library offers a number of iterator constructors making iterators
from different sources like strings, byte slices, byte readers, files and shell command outputs.
There is also a number of mapping and filtering functions, as well as writers for strings,
byte slices and files. All the above is composable using fluent API. For example, reading a file
line by line, removing leading and trailing whitespace from each line, selecting only non-empty
lines that also do not start from the character '#', and storing the result in a slice of strings,
may look like this:

      lines, err := strit.FromFile("some-file").
                    Map(bytes.TrimSpace).
                    Filter(strit.Not(strit.Empty).OrNot(strit.StartsWith("#"))).
                    Strings()

Here strit.FromFile is an iterator constructor, Map and Filter operations are iterator wrappers each
creating a new iterator from the existing one, and Strings() is the iterator invocation. Also notice the
use of the provided predicate combinators in the parameter to the Filter() function.

Given that each iterator is itself a function, other iterators can be derived from the existing ones.
For example, the following function takes an iterator and creates a new iterator that prepends the line
number to each line from the original iterator:

    func Numbered(iter Iter) Iter {
        return func(fn Func) error { // this is the new iterator function
            i := 0

            // this is the invocation of the original iterator function
            return iter(func(line []byte) error {
                i++
                line = []byte(fmt.Sprintf("%d %s", i, string(line)))
                return fn(line)	// invocation of the callback
            })
        }
    }

Iterators in this library are lazy, meaning that the actual iteration happens only when the iterator function is
called, not when it is created. Some of the provided iterators are reusable, meaning that such iterator may be
called more than once, but in general this property cannot be guaranteed for those sources that cannot be
read multiple times (like io.Reader), so in general it is advised that in the absence of any specific
knowledge every iterator should be treated as not reusable. All the iterator implementations have O(n)
complexity in time and O(1) in space.

One disadvantage of the suggested model is the complex implementation of parallel composition of iterators,
see the comment to the Merge() function for more details. Also, there is a certain cost of abstraction incurred,
in other words, a straight-forward implementation of the above example could be somewhat more efficient
in terms of CPU cycles and memory consumption, though the difference would mostly come from the extra
function calls and as such would probably be not so substantial, especially for i/o-bound sources.
The actual benchmaking of some simple processing scenarios shows that the performance difference is truly minor.
*/
package strit

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Func is the type of callback function used by Iter.
type Func func([]byte) error

// Iter is the iterator type.
type Iter func(Func) error

// Filter makes a new iterator that produces only the strings for which the supplied predicate returns 'true'.
func (iter Iter) Filter(pred Pred) Iter {
	return func(fn Func) error {
		return iter(func(line []byte) (err error) {
			if pred(line) {
				err = fn(line)
			}

			return
		})
	}
}

// Map makes a new iterator that applies the specified function to every input string.
func (iter Iter) Map(mapper func([]byte) []byte) Iter {
	return func(fn Func) error {
		return iter(func(line []byte) error {
			return fn(mapper(line))
		})
	}
}

// ErrSkip is a special error type to skip an item in GenMap() function.
var ErrSkip = errors.New("Item skipped")

// GenMap makes a new iterator that applies the supplied function to every input string.
// The function may return a non-nil error, in which case the iteration will stop and
// the error will be propagated back to the source. A special error value of ErrSkip instructs
// the iterator to simply skip the current string.
func (iter Iter) GenMap(mapper func([]byte) ([]byte, error)) Iter {
	return func(fn Func) error {
		return iter(func(line []byte) error {
			switch s, err := mapper(line); err {
			case nil:
				return fn(s)
			case ErrSkip:
				return nil
			default:
				return err
			}
		})
	}
}

// FirstNonEmpty makes a new iterator that skips everything until the supplied mapper function returns
// a non-empty slice, which gets passed down the pipeline, and then the iteration stops. This is
// essentially a search for the first non-empty string returned from the mapper.
func (iter Iter) FirstNonEmpty(mapper func([]byte) []byte) Iter {
	return func(fn Func) error {
		return iter(func(line []byte) (err error) {
			if line = mapper(line); len(line) > 0 {
				if err = fn(line); err == nil {
					err = io.EOF
				}
			}

			return
		})
	}
}

// TakeWhile makes a new iterator that produces its output while the specified predicate returns 'true'.
func (iter Iter) TakeWhile(pred Pred) Iter {
	return func(fn Func) error {
		return iter(func(line []byte) error {
			if pred(line) {
				return fn(line)
			}

			return io.EOF
		})
	}
}

// Take makes a new iterator that produces no more than the specified number of output strings.
func (iter Iter) Take(numLines uint64) Iter {
	return func(fn Func) error {
		count := numLines

		return iter(func(line []byte) error {
			if count == 0 {
				return io.EOF
			}

			count--
			return fn(line)
		})
	}
}

// SkipWhile makes a new iterator that skips input strings until the supplied predicate returns 'false'
// for the first time and passes the remaining strings to the callback function.
func (iter Iter) SkipWhile(pred Pred) Iter {
	return func(fn Func) error {
		skip := true

		return iter(func(line []byte) (err error) {
			if skip {
				skip = pred(line)
			}

			if !skip {
				err = fn(line)
			}

			return
		})
	}
}

// Skip makes a new iterator that skips the specified number of input strings and passes
// all the remaining strings to the callback function.
func (iter Iter) Skip(numLines uint64) Iter {
	return func(fn Func) error {
		count := numLines

		return iter(func(line []byte) error {
			if count == 0 {
				return fn(line)
			}

			count--
			return nil
		})
	}
}

// Chain implements sequential composition of its input iterators. The returned iterator
// invokes all the input iterators one after another, from left to the right.
func Chain(its ...Iter) Iter {
	switch len(its) {
	case 0:
		panic("No iterators in Chain() function")
	case 1:
		return its[0]
	}

	return func(fn Func) error {
		for _, iter := range its {
			if err := iter(fn); err != nil {
				return err
			}
		}

		return nil
	}
}

// Merge implements parallel composition of its input iterators. The returned iterator
// on each step invokes all its inputs in parallel, taking one string from each input iterator,
// and then joins those strings around the specified separator to produce one output string.
// The iteration stops when any input iterator gets exhausted.
// It should be noted that the iterator type used in this library provides for easy
// sequential composition of iterators, but the parallel composition is
// substantially more complex. In the current implementation each input iterator
// except the first one runs in a dedicated goroutine which pipes its output back through a channel.
// That complexity is usually not a problem if at least one of the input iterators is i/o-bound,
// but it is likely to become a performance bottleneck if all the iterators read from the memory.
// Please use this function responsibly.
func Merge(sep string, its ...Iter) Iter {
	// check the number of input iterators
	switch len(its) {
	case 0:
		panic("No iterators in Merge() function")
	case 1:
		return its[0]
	}

	// data structure for channels
	type cdata struct {
		str string
		err error
	}

	// the iterator
	return func(fn Func) error {
		// cancellation flag
		done := make(chan int)

		defer close(done)

		// number of goroutines
		n := len(its) - 1

		// channels for iterators
		chs := make([]chan cdata, n)

		// create channels and start goroutines
		for i := 0; i < n; i++ {
			chs[i] = make(chan cdata, 10) // not sure about the size...

			// start goroutine
			go func(iter Iter, ch chan<- cdata) {
				defer close(ch)

				// iterate and feed the channel
				err := iter(func(line []byte) error {
					select {
					case ch <- cdata{str: string(line)}:
						return nil
					case <-done:
						return errors.New("Interrupted")
					}
				})

				// post error, if any
				if err != nil {
					select {
					case ch <- cdata{err: err}:
					case <-done:
					}
				}
			}(its[i+1], chs[i])
		}

		// buffer for strings
		buff := make([]string, len(its))

		// invoke the first iterator
		return its[0](func(line []byte) error {
			// take the string from this iterator
			buff[0] = string(line)

			// take one item from every channel
			for i, ch := range chs {
				item, ok := <-ch

				if !ok {
					// iterator has finished
					return io.EOF
				}

				if item.err != nil {
					// iterator has failed
					return item.err
				}

				// store the string
				buff[i+1] = item.str
			}

			// merge strings and invoke the callback
			return fn([]byte(strings.Join(buff, sep)))
		})
	}
}

// String invokes the iterator and concatenates its output into one string.
func (iter Iter) String() (res string, err error) {
	var buff bytes.Buffer

	if _, err = iter.WriteSepTo(&buff, ""); err == nil {
		res = buff.String()
	}

	return
}

// Bytes invokes the iterator and concatenates its output into one byte slice.
func (iter Iter) Bytes() (res []byte, err error) {
	var buff bytes.Buffer

	if _, err = iter.WriteSepTo(&buff, ""); err == nil {
		res = buff.Bytes()
	}

	return
}

// Strings invokes the iterator and collects its output into a slice of strings.
func (iter Iter) Strings() (res []string, err error) {
	if err = iter(func(line []byte) error {
		res = append(res, string(line))
		return nil
	}); err != nil {
		res = nil
	}

	return
}

// Join invokes the iterator and collects its output into one string, delimited
// by the specified separator.
func (iter Iter) Join(sep string) (res string, err error) {
	var buff bytes.Buffer

	if _, err = iter.WriteSepTo(&buff, sep); err == nil {
		res = buff.String()
	}

	return
}

// JoinBytes invokes the iterator and collects its output into one byte slice, delimited
// by the specified separator.
func (iter Iter) JoinBytes(sep string) (res []byte, err error) {
	var buff bytes.Buffer

	if _, err = iter.WriteSepTo(&buff, sep); err == nil {
		res = buff.Bytes()
	}

	return
}

// WriteSepTo invokes the iterator and writes its output strings, delimited by the specified separator,
// into the specified Writer. The method returns the total number of bytes written, or an error.
func (iter Iter) WriteSepTo(dest io.Writer, sep string) (n int64, err error) {
	if len(sep) == 0 {
		// optimised version for empty separator
		err = iter(func(line []byte) error {
			num, e := dest.Write(line)
			n += int64(num)
			return e
		})

		return
	}

	// full version
	delim := []byte(sep)

	var lines int64

	err = iter(func(line []byte) (e error) {
		var num int

		// lines is only used for detecting the first write
		if lines++; lines != 1 {
			num, e = dest.Write(delim)
			n += int64(num)

			if err != nil {
				return
			}
		}

		num, e = dest.Write(line)
		n += int64(num)
		return
	})

	return
}

// WriteTo invokes the iterator and writes its output strings, delimited by new line character,
// into the specified Writer. The method returns the total number of bytes written, or an error.
func (iter Iter) WriteTo(dest io.Writer) (int64, error) {
	return iter.WriteSepTo(dest, "\n")
}

// WriteSepToFile creates a file with the specified name, and then invokes the iterator and writes
// its output strings, delimited by the specified separator, into the file. The method returns
// the total number of bytes written, or an error, in which case the resulting file gets deleted.
func (iter Iter) WriteSepToFile(name, sep string) (n int64, err error) {
	var file *os.File

	if file, err = os.Create(name); err != nil {
		return
	}

	defer func() {
		// close and delete the file if panicking
		if p := recover(); p != nil {
			file.Close()
			os.Remove(name)
			panic(p)
		}

		// close and set error
		if e := file.Close(); e != nil && err == nil {
			err = e
		}

		// delete the file on error
		if err != nil {
			os.Remove(name)
		}
	}()

	n, err = iter.WriteSepTo(file, sep)
	return
}

// WriteToFile creates a file with the specified name, and then invokes the iterator and writes
// its output strings, delimited by new line character, into the file. The method returns
// the total number of bytes written, or an error, in which case the resulting file gets deleted.
func (iter Iter) WriteToFile(name string) (int64, error) {
	return iter.WriteSepToFile(name, "\n")
}

// FromReaderSF constructs a new iterator that reads its input byte stream from the specified Reader and
// breaks the stream into tokens using the supplied split function. If the function is set to nil then
// the iterator breaks the input into lines with line termination stripped. Internally the iterator
// is implemented using bufio.Scanner, please refer to its documentation for more details on split functions.
func FromReaderSF(sf bufio.SplitFunc, input io.Reader) Iter {
	return func(fn Func) (err error) {
		if err = iterate(input, sf, fn); err == io.EOF {
			err = nil // io.EOF indicates early stop
		}

		return
	}
}

// FromReader constructs a new iterator that reads its input byte stream from the specified Reader and
// breaks the input into lines with line termination stripped.
func FromReader(input io.Reader) Iter {
	return FromReaderSF(nil, input)
}

// FromReadCloserSF is a wrapper around FromReaderSF with exactly the same functionality that also
// closes the input Reader at the end of the iteration.
func FromReadCloserSF(sf bufio.SplitFunc, input io.ReadCloser) Iter {
	return func(fn Func) error {
		defer input.Close()
		return FromReaderSF(sf, input)(fn)
	}
}

// FromReadCloser constructs a new iterator that reads its input byte stream from the specified Reader and
// breaks the input into lines with line termination stripped. The input Reader gets closed at the end
// of the iteration.
func FromReadCloser(input io.ReadCloser) Iter {
	return FromReadCloserSF(nil, input)
}

// FromFileSF constructs a new iterator that reads its input byte stream from the specified file and
// breaks the stream into tokens using the supplied split function. If the function is set to nil then
// the iterator breaks the input into lines with line termination stripped. Internally the iterator
// is implemented using bufio.Scanner, please refer to its documentation for more details on split functions.
func FromFileSF(sf bufio.SplitFunc, name string) Iter {
	return func(fn Func) error {
		file, err := os.Open(name)

		if err != nil {
			return err
		}

		return FromReadCloserSF(sf, file)(fn)
	}
}

// FromFile constructs a new iterator that reads its input byte stream from the specified file and
// breaks the stream into lines with line termination stripped.
func FromFile(name string) Iter {
	return FromFileSF(nil, name)
}

// FromDir creates a new iterator that reads all entries from the specified directory and produces
// the names of those entries for which the supplied predicate returns 'true'. If the predicate is
// set to nil then the iterator produces only regular files, directories and symlinks to files.
func FromDir(name string, pred func(os.FileInfo) bool) Iter {
	if pred == nil {
		pred = defaultFilePredicate
	}

	return func(fn Func) error {
		return readDir(fn, name, pred)
	}
}

// FromDirWalk creates a new iterator that wraps filepath.Walk recursive directory traversal function.
// The iterator produces names of filesystem entries in accordance with the supplied WalkFunc,
// please refer to the filepath.Walk documentation for more details. If the WalkFunc is set to nil
// then the default function will be used which accepts all the filesystem entries.
func FromDirWalk(root string, wf filepath.WalkFunc) Iter {
	if wf == nil {
		wf = defaultDirWalkFunc
	}

	return func(fn Func) (err error) {
		return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err = wf(path, info, err); err == nil {
				err = fn([]byte(path))
			}

			return err
		})
	}
}

// FromBytesSF constructs a new iterator that reads the specified byte slice and
// breaks it into tokens using the supplied split function. If the function is set to nil then
// the iterator breaks the input into lines with line termination stripped. Internally the iterator
// is implemented using bufio.Scanner, please refer to its documentation for more details on split functions.
func FromBytesSF(sf bufio.SplitFunc, src []byte) Iter {
	return func(fn Func) (err error) {
		if err = iterate(bytes.NewBuffer(src), sf, fn); err == io.EOF {
			err = nil // io.EOF is not an error
		}

		return
	}
}

// FromBytes constructs a new iterator that reads the specified byte slice and
// breaks it into lines with line termination stripped.
func FromBytes(src []byte) Iter {
	return FromBytesSF(nil, src)
}

// FromStringSF constructs a new iterator that reads the specified string and
// breaks it into tokens using the supplied split function. If the function is set to nil then
// the iterator breaks the input into lines with line termination stripped. Internally the iterator
// is implemented using bufio.Scanner, please refer to its documentation for more details on split functions.
func FromStringSF(sf bufio.SplitFunc, src string) Iter {
	return func(fn Func) (err error) {
		if err = iterate(bytes.NewBufferString(src), sf, fn); err == io.EOF {
			err = nil // io.EOF is not an error
		}

		return
	}
}

// FromString constructs a new iterator that reads the specified string and
// breaks it into lines with line termination stripped.
func FromString(src string) Iter {
	return FromStringSF(nil, src)
}

// FromStrings constructs a new iterator that reads the specified slice of strings, one string at a time.
func FromStrings(src []string) Iter {
	return func(fn Func) (err error) {
		for _, s := range src {
			if err = fn([]byte(s)); err != nil {
				break
			}
		}

		if err == io.EOF { // io.EOF is not an error
			err = nil
		}

		return
	}
}

// FromCommandSF constructs a new iterator that invokes the specified shell command,
// reads the command output (stdout) and breaks it into tokens using the supplied split function.
// If the function is set to nil then the iterator breaks the command output into lines with line termination
// stripped. Internally the iterator is implemented using bufio.Scanner, please refer to its
// documentation for more details on split functions.
func FromCommandSF(sf bufio.SplitFunc, name string, args ...string) Iter {
	return func(fn Func) error {
		// command
		cmd := exec.Command(name, args...)

		// command's stdout
		cmdOutput, err := cmd.StdoutPipe()

		if err != nil {
			return err
		}

		// buffer for command's stderr
		var stderr bytes.Buffer

		cmd.Stderr = &stderr

		// start command
		if err = cmd.Start(); err != nil {
			return err
		}

		// iterate over the output
		if err = iterate(cmdOutput, sf, fn); err != nil {
			// the process is no longer needed, stop it
			go func() {
				cmd.Process.Signal(os.Interrupt) // try SIGINT first

				t := time.AfterFunc(5*time.Second, func() {
					cmd.Process.Kill()
				})

				defer t.Stop()

				// from the documentation (https://golang.org/pkg/os/exec/#Cmd.StdoutPipe):
				// 	"it is incorrect to call Wait before all reads from the pipe have completed"
				io.Copy(ioutil.Discard, cmdOutput)

				// wait for completion
				cmd.Wait()
			}()

			// return the error
			if err == io.EOF {
				err = nil
			}

			return err
		}

		// wait for completion
		if err = cmd.Wait(); err != nil {
			// try to replace the error with command's stderr
			if _, ok := err.(*exec.ExitError); ok && stderr.Len() > 0 {
				err = errors.New(string(bytes.TrimSpace(stderr.Bytes())))
			}
		}

		return err
	}
}

// FromCommand constructs a new iterator that invokes the specified shell command,
// reads the command output (stdout) and breaks it into lines with line termination stripped.
func FromCommand(name string, args ...string) Iter {
	return FromCommandSF(nil, name, args...)
}

// ScanNullTerminatedStrings is a split function that splits input on null bytes. Useful mostly
// with FromCommand* functions, in cases where the invoked command generates null-terminated
// strings, like 'find ... -print0'.
func ScanNullTerminatedStrings(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	if i := bytes.IndexByte(data, 0); i >= 0 {
		// got the string
		return i + 1, data[0:i], nil
	}

	// last string must be null-terminated
	if atEOF {
		return 0, nil, errors.New("Last string is not null-terminated")
	}

	return 0, nil, nil
}

// Pred is the type of string predicate. The type has a number of combining methods allowing for
// convenient composition of predicate functions, for example:
//    strit.Not(strit.Empty).AndNot(strit.StartsWith("#"))
// or
//    strit.StartsWith("xyz").Or(strit.StartsWith("abc"))
type Pred func([]byte) bool

// And is a predicate combinator. It creates a new predicate that applies logical 'and'
// to the original and the supplied predicates.
func (orig Pred) And(other Pred) Pred {
	return func(line []byte) bool {
		return orig(line) && other(line)
	}
}

// AndNot is a predicate combinator. It creates a new predicate that returns 'true' only if
// the original returns 'true' and the other predicate returns 'false'.
func (orig Pred) AndNot(next Pred) Pred {
	return func(line []byte) bool {
		return orig(line) && !next(line)
	}
}

// Or is a predicate combinator. It creates a new predicate that applies logical 'or'
// to the original and the supplied predicates.
func (orig Pred) Or(other Pred) Pred {
	return func(line []byte) bool {
		return orig(line) || other(line)
	}
}

// OrNot is a predicate combinator. It creates a new predicate that returns 'true' if
// either the original returns 'true' or the other predicate returns 'false'.
func (orig Pred) OrNot(other Pred) Pred {
	return func(line []byte) bool {
		return orig(line) || !other(line)
	}
}

// Not is a predicate wrapper that returns a new predicate negating the result of the supplied predicate.
func Not(pred Pred) Pred {
	return func(line []byte) bool {
		return !pred(line)
	}
}

// Empty is a predicate that returns 'true' only if the input string is empty.
func Empty(line []byte) bool {
	return len(line) == 0
}

// StartsWith is a predicate that returns 'true' if the input string has the specified prefix.
func StartsWith(prefix string) Pred {
	return func(line []byte) bool {
		return bytes.HasPrefix(line, []byte(prefix))
	}
}

// EndsWith is a predicate that returns 'true' if the input string has the specified suffix.
func EndsWith(prefix string) Pred {
	return func(line []byte) bool {
		return bytes.HasSuffix(line, []byte(prefix))
	}
}

// helper functions -------------------------------------------------------------------------------
// iterator core
func iterate(input io.Reader, sf bufio.SplitFunc, fn Func) error {
	src := bufio.NewScanner(input)

	if sf != nil {
		src.Split(sf)
	}

	for src.Scan() {
		if err := fn(src.Bytes()); err != nil {
			return err // returns io.EOF when stopped early
		}
	}

	return src.Err() // returns nil on EOF
}

// directory read implementation
func readDir(fn Func, dirName string, pred func(os.FileInfo) bool) error {
	// open directory
	dir, err := os.Open(dirName)

	if err != nil {
		return err
	}

	defer dir.Close()

	// get list of all items
	items, err := dir.Readdir(0)

	if err != nil {
		return err
	}

	// select items and invoke callback
	for _, item := range items {
		if pred(item) {
			if err := fn([]byte(filepath.Join(dirName, item.Name()))); err != nil {
				return err
			}
		}
	}

	return nil
}

// default FromDir() predicate selects only regular files, directories and symlinks
func defaultFilePredicate(info os.FileInfo) bool {
	return info.IsDir() || info.Mode().IsRegular() || info.Mode()&os.ModeType == os.ModeSymlink
}

// defaultDirWalkFunc is the default function for FromDirWalk() constructor. It accepts all the entries.
func defaultDirWalkFunc(_ string, _ os.FileInfo, _ error) error { return nil }
