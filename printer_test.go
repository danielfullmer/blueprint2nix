// This is a modified version of github/google/blueprint/parser for blueprint2nix
// Originally from revision: ba1ea7583953186a1a5519c0cd1087e403ad516f

// Copyright 2014 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"testing"

	"github.com/google/blueprint/parser"
)

var validPrinterTestCases = []struct {
	input  string
	output string
}{
	{
		input: `
foo {}
`,
		output: `
{ foo }:
let

_missingName = foo {};

in { inherit _missingName; }
`,
	},
	{
		input: `
foo(name= "abc",num= 4,)
`,
		output: `
{ foo }:
let

abc = foo {
    name = "abc";
    num = 4;
};

in { inherit abc; }
`,
	},
	{
		input: `
			foo {
				stuff: ["asdf", "jkl;", "qwert",
					"uiop", "bnm,"]
			}
			`,
		output: `
{ foo }:
let

_missingName = foo {
    stuff = [
        "asdf"
        "bnm,"
        "jkl;"
        "qwert"
        "uiop"
    ];
};

in { inherit _missingName; }
`,
	},
	// blueprint2nix: the input in upstream is invalid for the next few cases.
	// It appends a list with a string, evaluateOperator() says you can't add mismatched types
	// I've just fixed by making 'var' a list
	{
		input: `
		        var = ["asdf"]
			foo {
				stuff: ["asdf"] + var,
			}`,
		output: `
{ foo }:
let

var = ["asdf"];
_missingName = foo {
    stuff = ["asdf"] ++ var;
};

in { inherit _missingName; }
`,
	},
	// blueprint2nix: the input in upstream is invalid.
	{
		input: `
		        var = ["asdf"]
			foo {
				stuff: [
				    "asdf"
				] + var,
			}`,
		output: `
{ foo }:
let

var = ["asdf"];
_missingName = foo {
    stuff = [
        "asdf"
    ] ++ var;
};

in { inherit _missingName; }
`,
	},
	{
		input: `
		        var = ["asdf"]
			foo {
				stuff: ["asdf"] + var + ["qwert"],
			}`,
		output: `
{ foo }:
let

var = ["asdf"];
_missingName = foo {
    stuff = ["asdf"] ++ var ++ ["qwert"];
};

in { inherit _missingName; }
`,
	},
	{
		input: `
		foo {
			stuff: {
				isGood: true,
				name: "bar",
				num: 4,
			}
		}
		`,
		output: `
{ foo }:
let

_missingName = foo {
    stuff = {
        isGood = true;
        name = "bar";
        num = 4;
    };
};

in { inherit _missingName; }
`,
	},
	{
		input: `
// comment1
foo {
	// comment2
	isGood: true,  // comment3
}
`,
		output: `
{ foo }:
let

#  comment1
_missingName = foo {
    #  comment2
    isGood = true; #  comment3
};

in { inherit _missingName; }
`,
	},
	{
		input: `
foo {
	name: "abc",
	num: 4,
}

bar  {
	name: "def",
	num: 5,
}
		`,
		output: `
{ bar, foo }:
let

abc = foo {
    name = "abc";
    num = 4;
};

def = bar {
    name = "def";
    num = 5;
};

in { inherit abc def; }
`,
	},
	{
		input: `
foo {
    bar: "b" +
        "a" +
	"z",
}
`,
		output: `
{ foo }:
let

_missingName = foo {
    bar = "b" +
        "a" +
        "z";
};

in { inherit _missingName; }
`,
	},
	{
		input: `
foo = "stuff"
bar = foo
baz = foo + bar
// blueprint2nix unsupported: baz += foo
`,
		output: `
{ }:
let

foo = "stuff";
bar = foo;
baz = foo + bar;
#  blueprint2nix unsupported: baz += foo

in { }
`,
	},
	{
		input: `
foo = 100
bar = foo
baz = foo + bar
// blueprint2nix unsupported: baz += foo
`,
		output: `
{ }:
let

foo = 100;
bar = foo;
baz = foo + bar;
#  blueprint2nix unsupported: baz += foo

in { }
`,
	},
	{
		input: `
foo = "bar " +
    "" +
    "baz"
`,
		output: `
{ }:
let

foo = "bar " +
    "" +
    "baz";

in { }
`,
	},
	{
		input: `
//test
test /* test */ {
    srcs: [
        /*"bootstrap/bootstrap.go",
    "bootstrap/cleanup.go",*/
        "bootstrap/command.go",
        "bootstrap/doc.go", //doc.go
        "bootstrap/config.go", //config.go
    ],
    deps: ["libabc"],
    incs: []
} //test
//test
test2 {
}


//test3
`,
		output: `
{ test, test2 }:
let

# test
_missingName = test /* test */ {
    srcs = [
        /*"bootstrap/bootstrap.go",
        "bootstrap/cleanup.go",*/
        "bootstrap/command.go"
        "bootstrap/config.go" # config.go
        "bootstrap/doc.go" # doc.go
    ];
    deps = ["libabc"];
    incs = [];
}; # test
# test

_missingName = test2 {
};

# test3

in { inherit _missingName; }
`,
	},
	{
		input: `
// test
module // test

 {
    srcs
   : [
        "src1.c",
        "src2.c",
    ],
//test
}
//test2
`,
		output: `
{ module }:
let

#  test
_missingName = module { #  test

    srcs = [
        "src1.c"
        "src2.c"
    ];
    # test
};

# test2

in { inherit _missingName; }
`,
	},
	{
		input: `
/*test {
    test: true,
}*/

test {
/*test: true,*/
}

// This
/* Is *//* A */ // A
// A

// Multiline
// Comment

test {}

// This
/* Is */
// A
// Trailing

// Multiline
// Comment
`,
		output: `
{ test }:
let

/*test {
    test: true,
}*/

_missingName = test {
    /*test: true,*/
};

#  This
/* Is */ /* A */ #  A
#  A

#  Multiline
#  Comment

_missingName = test {};

#  This
/* Is */
#  A
#  Trailing

#  Multiline
#  Comment

in { inherit _missingName; }
`,
	},
	{
		input: `
test // test

// test
{
}
`,
		output: `
{ test }:
let

_missingName = test { #  test

#  test

};

in { inherit _missingName; }
`,
	},
}

func TestPrinter(t *testing.T) {
	for _, testCase := range validPrinterTestCases {
		in := testCase.input[1:]
		expected := testCase.output[1:]

		r := bytes.NewBufferString(in)
		file, errs := parser.ParseAndEval("", r, parser.NewScope(nil)) // blueprint2nix: we have to evaluate as well to know what nix operators to convert to
		if len(errs) != 0 {
			t.Errorf("test case: %s", in)
			t.Errorf("unexpected errors:")
			for _, err := range errs {
				t.Errorf("  %s", err)
			}
			t.FailNow()
		}

		parser.SortLists(file)

		got, err := NixPrint(file)
		if err != nil {
			t.Errorf("test case: %s", in)
			t.Errorf("unexpected error: %s", err)
			t.FailNow()
		}

		if string(got) != expected {
			t.Errorf("test case: %s", in)
			t.Errorf("  expected: %s", expected)
			t.Errorf("       got: %s", string(got))
		}
	}
}
