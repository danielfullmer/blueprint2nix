package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/google/blueprint/parser"
)

func convertFile(path string) {
	f, _ := os.Open(path)
	src, _ := ioutil.ReadAll(f)
	r := bytes.NewBuffer(src)

	// Need to evaluate file as well as parse so that it knows types to properly convert "+" operator.
	file, _ := parser.ParseAndEval(path, r, parser.NewScope(nil))
	out, _ := NixPrint(file)

	outpath := strings.TrimSuffix(path, ".bp")
	ioutil.WriteFile(outpath+".nix", out, 0644)
}

func main() {
	flag.Parse()

	for i := 0; i < flag.NArg(); i++ {
		path := flag.Arg(i)
		fmt.Println("Converting", path)
		convertFile(path)
	}
}
