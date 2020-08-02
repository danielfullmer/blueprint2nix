package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/parser"
)

func IsValidNixIdentifier(name string) bool {
	matched, _ := regexp.MatchString("^[a-zA-Z_][a-zA-Z0-9_'-]*$", name)
	return matched
}

func convertFile(inpath string, outpath string) []string {
	f, _ := os.Open(inpath)
	src, _ := ioutil.ReadAll(f)
	r := bytes.NewBuffer(src)

	// Need to evaluate file as well as parse so that it knows types to properly convert "+" operator.
	file, err := parser.ParseAndEval(inpath, r, parser.NewScope(nil))
	if err != nil {
		panic("file doesnt parse")
	}

	out, _ := NixPrint(file)

	ioutil.WriteFile(outpath, out, 0644)

	return GetModulesInfo(file.Defs, ModuleName)
}

func nixFilePath(inpath string) string {
	outpath := strings.ReplaceAll(inpath, "/", "_")
	outpath = strings.TrimSuffix(outpath, "_Android.bp")
	outpath += ".nix"
	return outpath
}

// See ParseFileList for reference
func processDir(rootdir string) {
	// Could probably use something faster from soong
	fmt.Println("Scanning for Android.bp files")
	filePaths := make([]string, 0)
	filepath.Walk(rootdir, func(path string, f os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(f.Name(), "Android.bp") {
			// TODO: Temporary hack until we have soong namespaces
			if strings.Contains(path, "device/") {
				return nil
			}
			if strings.Contains(path, "hardware/") {
				return nil
			}
			if strings.Contains(path, "prebuilts/vndk/") {
				return nil
			}

			filePaths = append(filePaths, path)
		}
		return nil
	})

	type moduleNamePair struct {
		fileName    string
		moduleNames []string
	}

	moduleNamesCh := make(chan moduleNamePair)
	fileHandler := func(file *parser.File) {
		fmt.Println("Converting", file.Name)
		nixCode, _ := NixPrint(file)

		ioutil.WriteFile("out/"+nixFilePath(file.Name), nixCode, 0644)

		moduleNamesCh <- moduleNamePair{
			fileName:    file.Name,
			moduleNames: GetModulesInfo(file.Defs, ModuleName),
		}
	}

	ctx := blueprint.NewContext()

	doneCh := make(chan struct{})
	go func() {
		ctx.WalkBlueprintsFiles(rootdir, filePaths, fileHandler)
		doneCh <- struct{}{}
	}()

	moduleNamesMap := make(map[string][]string)
	moduleNamesSet := make(map[string]bool)
	fileNames := make([]string, 0)
loop:
	for {
		select {
		case m := <-moduleNamesCh:
			// check for duplicated names
			for _, name := range m.moduleNames {
				if _, ok := moduleNamesSet[name]; ok {
					fmt.Fprintln(os.Stderr, "Duplicate assigned name", name, "in", m.fileName, "skipping file")
					continue loop
				} else {
					moduleNamesSet[name] = true
				}
			}

			fileNames = append(fileNames, m.fileName)
			sort.Strings(m.moduleNames)
			moduleNamesMap[m.fileName] = m.moduleNames
		case <-doneCh:
			break loop
		}
	}
	sort.Strings(fileNames)

	fmt.Println("Writing blueprint-packages.nix")
	f, _ := os.Create("out/blueprint-packages.nix")
	f.WriteString("{ callBPPackage }:\n")
	f.WriteString("{\n")
	for _, fileName := range fileNames {
		names := moduleNamesMap[fileName]
		if len(names) > 0 {
			f.WriteString("  inherit (callBPPackage \"" + filepath.Dir(fileName) + "\" ./" + nixFilePath(fileName) + " {})\n")
			f.WriteString("    ")
			f.WriteString(strings.Join(names, " "))
			f.WriteString(";\n\n")
		}
	}
	f.WriteString("}\n")
}

func main() {
	flag.Parse()

	for i := 0; i < flag.NArg(); i++ {
		path := flag.Arg(i)
		switch dir, err := os.Stat(path); {
		case err != nil:
			fmt.Fprintln(os.Stderr, err)
		case dir.IsDir():
			processDir(path)
		default:
			fmt.Println("Converting", path)
			convertFile(path, strings.TrimSuffix(path, ".bp")+".nix")
		}
	}
}
