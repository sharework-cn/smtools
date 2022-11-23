package utils_test

import (
	"fmt"
	"net/http"
	"testing"
)

type gf func(data ...any) (any, error)
type f struct {
	ss *[]string
}

func TestF(t *testing.T) {
	d := func(data ...any) (any, error) {
		return "", nil
	}
	k := http.Client{}

	c := func(name string, age int) (string, error) {
		return fmt.Sprintf(name, age), nil
	}
	d("my %s", 25)
	c("my %s", 25)

	k := f{nil}
	for i, v := range *k.ss {
		print(i, v)
	}
	return
}
