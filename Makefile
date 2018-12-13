SRC = strit.go  strit_test.go

.PHONY : build test check

build : check
	go build

test : check
	go test

check : $(SRC)
	goimports -w $^
