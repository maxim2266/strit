# strit

[![GoDoc](https://godoc.org/github.com/maxim2266/strit?status.svg)](https://godoc.org/github.com/maxim2266/strit)
[![Go report](http://goreportcard.com/badge/maxim2266/strit)](http://goreportcard.com/report/maxim2266/strit)

Package `strit` (STRing ITerator) assists in development of string processing pipelines by providing a simple
iteration model that allows for easy composition of processing stages.

### Motivation
Suppose we want to develop a function that reads a file line by line, removes leading and trailing
whitespace from each line, selects only non-empty lines that also do not start with the `#` symbol, and
stores those lines in a slice of strings. Using the Go standard library one possible implementation
of the function may look like this:
```Go
func ReadConfig(fileName string) ([]string, error) {
	file, err := os.Open(fileName)

	if err != nil {
		return nil, err
	}

	defer file.Close()

	var res []string
	src := bufio.NewScanner(file)

	for src.Scan() {
		line := bytes.TrimSpace(src.Bytes())

		if len(line) > 0 && line[0] != '#' {
			res = append(res, string(line))
		}
	}

	if err = src.Err(); err != nil {
		return nil, err
	}

	return res, nil
}
```
Using `strit` package the implementation can be simplified down to:
```Go
func ReadConfig(fileName string) ([]string, error) {
	return strit.FromFile(fileName).
           Map(bytes.TrimSpace).
           Filter(strit.Not(strit.Empty).AndNot(strit.StartsWith("#"))).
           Strings()
```
### Features
* A number of iterator constructors for reading text from a variety of sources:
  * `io.Reader`:
  	[`FromReader`](https://godoc.org/github.com/maxim2266/strit#FromReader)
  	[`FromReaderSF`](https://godoc.org/github.com/maxim2266/strit#FromReaderSF)
  * `io.ReadCloser`:
  	[`FromReadCloser`](https://godoc.org/github.com/maxim2266/strit#FromReadCloser)
  	[`FromReadCloserSF`](https://godoc.org/github.com/maxim2266/strit#FromReadCloserSF)
  * `[]byte`:
  	[`FromBytes`](https://godoc.org/github.com/maxim2266/strit#FromBytes)
  	[`FromBytesSF`](https://godoc.org/github.com/maxim2266/strit#FromBytesSF)
  * `string`:
  	[`FromString`](https://godoc.org/github.com/maxim2266/strit#FromString)
  	[`FromStringSF`](https://godoc.org/github.com/maxim2266/strit#FromStringSF)
  * `[]string`:
  	[`FromStrings`](https://godoc.org/github.com/maxim2266/strit#FromStrings)
  * Disk file:
  	[`FromFile`](https://godoc.org/github.com/maxim2266/strit#FromFile)
  	[`FromFileSF`](https://godoc.org/github.com/maxim2266/strit#FromFileSF)
  * Directory listing:
  	[`FromDir`](https://godoc.org/github.com/maxim2266/strit#FromDir)
  * Recursive directory listing:
  	[`FromDirWalk`](https://godoc.org/github.com/maxim2266/strit#FromDirWalk)
  * Shell command output:
  	[`FromCommand`](https://godoc.org/github.com/maxim2266/strit#FromCommand)
  	[`FromCommandSF`](https://godoc.org/github.com/maxim2266/strit#FromCommandSF)
* Mapping and filtering primitives:
	[`Filter`](https://godoc.org/github.com/maxim2266/strit#Iter.Filter)
	[`GenMap`](https://godoc.org/github.com/maxim2266/strit#Iter.GenMap)
	[`Map`](https://godoc.org/github.com/maxim2266/strit#Iter.Map)
* Sequence limiting functions:
	[`Skip`](https://godoc.org/github.com/maxim2266/strit#Iter.Skip)
	[`SkipWhile`](https://godoc.org/github.com/maxim2266/strit#Iter.SkipWhile)
	[`Take`](https://godoc.org/github.com/maxim2266/strit#Iter.Take)
	[`TakeWhile`](https://godoc.org/github.com/maxim2266/strit#Iter.TakeWhile)
* Search function:
	[`FirstNonEmpty`](https://godoc.org/github.com/maxim2266/strit#Iter.FirstNonEmpty)
* Piping iterator output through an external shell command:
	[`Pipe`](https://godoc.org/github.com/maxim2266/strit#Iter.Pipe)
	[`PipeSF`](https://godoc.org/github.com/maxim2266/strit#Iter.PipeSF)
* Iterator chaining (sequential combination):
	[`Chain`](https://godoc.org/github.com/maxim2266/strit#Chain)
* Iterator merging (parallel combination):
	[`Merge`](https://godoc.org/github.com/maxim2266/strit#Merge)
* Output collectors that invoke the given iterator and write the result to various destinations:
  * `string`:
  	[`String`](https://godoc.org/github.com/maxim2266/strit#Iter.String)
  	[`Join`](https://godoc.org/github.com/maxim2266/strit#Iter.Join)
  * `[]string`:
  	[`Strings`](https://godoc.org/github.com/maxim2266/strit#Iter.Strings)
  * `[]byte`:
  	[`Bytes`](https://godoc.org/github.com/maxim2266/strit#Iter.Bytes)
  	[`JoinBytes`](https://godoc.org/github.com/maxim2266/strit#Iter.JoinBytes)
  * `io.Writer`:
  	[`WriteTo`](https://godoc.org/github.com/maxim2266/strit#Iter.WriteTo)
  	[`WriteSepTo`](https://godoc.org/github.com/maxim2266/strit#Iter.WriteSepTo)
  * Disk file:
  	[`WriteToFile`](https://godoc.org/github.com/maxim2266/strit#Iter.WriteToFile)
  	[`WriteSepToFile`](https://godoc.org/github.com/maxim2266/strit#Iter.WriteSepToFile)
* Predicates and predicate combinators for use with `Filter`:
	[`Empty`](https://godoc.org/github.com/maxim2266/strit#Empty)
	[`StartsWith`](https://godoc.org/github.com/maxim2266/strit#StartsWith)
	[`EndsWith`](https://godoc.org/github.com/maxim2266/strit#EndsWith)
	[`Not`](https://godoc.org/github.com/maxim2266/strit#Not)
	[`And`](https://godoc.org/github.com/maxim2266/strit#Pred.And)
	[`AndNot`](https://godoc.org/github.com/maxim2266/strit#Pred.AndNot)
	[`Or`](https://godoc.org/github.com/maxim2266/strit#Pred.Or)
	[`OrNot`](https://godoc.org/github.com/maxim2266/strit#Pred.OrNot)
* Basic parsing supported via [`Parse`](https://godoc.org/github.com/maxim2266/strit#Iter.Parse) function.

### More examples:
* Naïve `grep`:
```Go
func main() {
	_, err := strit.FromReader(os.Stdin).
			Filter(regexp.MustCompile(os.Args[1]).Match).
			WriteSepTo(os.Stdout, "\n")

	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
```
* Recursively find all the filesystem entries matching the given regular expression:
```Go
func selectEntries(root string, re *regexp.Regexp) ([]string, error) {
	return FromDirWalk(root, nil).Filter(re.Match).Strings()
}
```
* Build a list of `.flac` files in the given directory, annotating each name with its corresponding
track number from FLAC metadata:
```Go
func namesWithTrackNumbers(dir string) ([]string, error) {
	return strit.FromDir(dir, func(info os.FileInfo) bool { return info.Mode().IsRegular() }).
		Filter(strit.EndsWith(".flac")).
		GenMap(prependTrackNo).
		Strings()
}

func prependTrackNo(file []byte) ([]byte, error) {
	name := string(file)

	no, err := strit.FromCommand("metaflac", "--list", "--block-type=VORBIS_COMMENT", name).
		FirstNonEmpty(func(s []byte) []byte {
			if m := trackNoRegex.FindSubmatch(s); len(m) == 2 {
				return m[1]
			}

			return nil
		}).
		String()

	if err != nil {
		return nil, err
	}

	if len(no) == 0 {
		return []byte("???: " + filepath.Base(name)), nil
	}

	return []byte(no + ": " + filepath.Base(name)), nil
}

var trackNoRegex = regexp.MustCompile(`tracknumber=([[:digit:]]+)$`)
```

### Project status
The project is in a beta state. Tested on Linux Mint 18 (based on Ubuntu 16.04). Go version 1.8.
Should also work on other platforms supported by Go runtime, but currently this is not tested.

##### License: BSD
