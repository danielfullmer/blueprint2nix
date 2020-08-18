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
	"github.com/google/blueprint/pathtools"
)

func IsValidNixIdentifier(name string) bool {
	matched, _ := regexp.MatchString("^[a-zA-Z_][a-zA-Z0-9_'-]*$", name)
	return matched
}

// Expand globs in srcs = [ ... ] of modules.
// TODO: Follow parse tree if "srcs" is not a list of literals but rather an expression
// TODO: Add a test here
func expandModuleSrcGlobs(rootdir string, file *parser.File) {
	for _, def := range file.Defs {
		if m, ok := def.(*parser.Module); ok {
			for _, prop := range m.Properties {
				if strings.HasSuffix(prop.Name, "srcs") || strings.HasSuffix(prop.Name, "dirs") || strings.HasSuffix(prop.Name, "data") || strings.HasSuffix(prop.Name, "files") {
					if l, ok := prop.Value.(*parser.List); ok {
						basepath := filepath.Join(rootdir, filepath.Dir(file.Name))
						expandListSrcGlobs(basepath, l)
					}
				}
			}
		}
	}
}

func expandListSrcGlobs(basepath string, l *parser.List) {
	newElements := make([]parser.Expression, 0)
	for _, v := range l.Values {
		if s, ok := v.(*parser.String); ok {
			if strings.Contains(s.Value, "*") {
				path := filepath.Join(basepath, s.Value)
				matches, _, _ := pathtools.Glob(path, []string{}, pathtools.FollowSymlinks)

				//if len(matches) == 0 {
				//	fmt.Println("No glob matches found for", path)
				//}

				for i, match := range matches {
					match = match[len(basepath)+1:]

					q := &parser.String{}
					q.Value = match
					if i == 0 {
						q.LiteralPos = s.LiteralPos
					} else {
						q.LiteralPos = noPos
					}

					newElements = append(newElements, q)
				}
			} else {
				newElements = append(newElements, v)
			}
		} else {
			newElements = append(newElements, v)
		}
	}
	l.Values = newElements
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

	expandModuleSrcGlobs("", file)
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
		expandModuleSrcGlobs(rootdir, file)
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
			uniqueNames := make([]string, 0)
			for _, name := range m.moduleNames {
				if _, ok := moduleNamesSet[name]; ok {
					fmt.Fprintln(os.Stderr, "Duplicate assigned name", name, "in", m.fileName)
				} else {
					moduleNamesSet[name] = true
					uniqueNames = append(uniqueNames, name)
				}
			}

			if len(uniqueNames) > 0 {
				fileNames = append(fileNames, m.fileName)
				sort.Strings(uniqueNames)
				moduleNamesMap[m.fileName] = uniqueNames
			}
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
			os.MkdirAll("out", 0755)
			processDir(path)
		default:
			fmt.Println("Converting", path)
			convertFile(path, strings.TrimSuffix(path, ".bp")+".nix")
		}
	}
}
