package config

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

// defaultPatternCases are lines each default pattern should find, and
// lines like them that no pattern of the language should. Every default
// pattern needs a case, so a new one comes with its tests. Each line is
// also tried in the variants lineVariants makes from it.
var defaultPatternCases = map[string]struct {
	file      string // a file of the language
	statement string // another statement, to share a line with each case
	hits      map[string][]string
	misses    []string
}{
	"go": {
		file:      "main.go",
		statement: "x := 1;",
		hits: map[string][]string{
			"fmt-print": {
				`fmt.Println("here", x)`, `	fmt.Printf("%v\n", x)`, `fmt.Print(x)`, `defer fmt.Println("done")`,
				`fmt.Println(x); x++`,
			},
			"builtin-print": {`println("here")`, `	print(x)`, `println(a); println(b)`, `defer func() { println("done") }()`},
			"spew":          {`spew.Dump(x)`, `spew.Printf("%v", x)`, `s := spew.Sdump(x)`},
			"litter":        {`litter.Dump(x)`, `s := litter.Sdump(x)`},
		},
		misses: []string{
			`fmt.Fprintln(w, x)`, `fmt.Sprintf("%d", n)`, `fmt.Errorf("x: %w", err)`, `log.Printf("x")`,
			`t.Logf("x")`, `w.Println(x)`, `sprint(x)`, `blueprint(x)`, `printer.Print(x)`, `spewer.Dump(x)`,
			`litter.Options{}`, `x := fmt.Sprint(y); w.Println(x)`, `func print(x any) {`, `func println(a ...any) {`,
			`func (p *printer) print(s string) {`, `func (p printer) println() {`,
		},
	},
	"javascript": {
		file:      "app.ts",
		statement: `let a = "is valid js";`,
		hits: map[string][]string{
			"console": {
				`console.log(x)`, `  console.debug("a")`, `console.table(rows)`, `console.trace()`, `console.dir(obj)`,
				`if (x) console.log(x);`, `console.log(a);console.log(b);`,
			},
			"debugger": {`debugger;`, `    debugger`, `if (x) debugger;`, `debugger; foo();`, `{ debugger }`},
		},
		misses: []string{
			`console.error(err)`, `console.warn("x")`, `myconsole.log(x)`, `logger.log(x)`, `console.logger(x)`,
			`const debuggerEnabled = true`, `x.debugger = true`, `const o = { debugger: true };`, `log("debugger")`,
			`debuggers.push(x)`,
		},
	},
	"php": {
		file:      "index.php",
		statement: `$msg = "is valid php";`,
		hits: map[string][]string{
			"var-dump": {`var_dump($x);`, `\var_dump($x);`, `<?php var_dump($x); ?>`, `var_dump($x); $y = 2;`},
			"print-r":  {`print_r($x);`, `echo print_r($x, true);`},
			"dump-die": {
				`dd($x);`, `\dd($x);`, `@dd($x)`, `dump($request);`, `ddd($x);`, `dd ($x);`, `return dd($x);`,
				`$msg = "is valid php";dd ('heres a msg');`, `$y = $x;dd($x);dump($y);`,
				`$f = function ($x) { dd($x); };`, `$f = fn ($x) => dump($x);`,
			},
			"ray":          {`ray($x);`, `ray($x)->green();`, `\ray($x);`, `$f = function () { ray(1); };`},
			"xdebug-break": {`xdebug_break();`},
		},
		misses: []string{
			`$this->dump($x);`, `$obj?->dump($x);`, `Foo::dump($x);`, `static::dd($x);`, `$dump($x);`, `odd($x);`,
			`add($x);`, `$this->dd($x);`, `$array = array(1);`, `$this->ray(1);`, `var_export($x, true);`,
			`$logger->debug($x);`, `$print_r = 1;`, `dumper($x);`, `$msg = "is valid php";add ('heres a msg');`,
			`$a = add($b);$c = odd ($d);`, `function dump($x) {`, `public function dd(...$args): never`,
			`public static function dump($value)`, `abstract public function dump();`, `function &dump(array $x)`,
			`function ray(...$items) {`,
		},
	},
}

// lineVariants returns line as written, with a space before each "(", and
// both of those after another statement on the same line, with no space
// between them.
func lineVariants(line, statement string) []string {
	spaced := strings.ReplaceAll(line, "(", " (")
	return []string{line, spaced, statement + line, statement + spaced}
}

func TestDefaultPatterns(t *testing.T) {
	f, err := parse(defaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := slices.Sorted(maps.Keys(f.Languages)), slices.Sorted(maps.Keys(defaultPatternCases)); !slices.Equal(got, want) {
		t.Fatalf("default languages = %v, but cases are for %v", got, want)
	}
	rules, err := compile(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	for language, c := range defaultPatternCases {
		t.Run(language, func(t *testing.T) {
			byName := map[string]Rule{}
			for _, rule := range rules.For(c.file) {
				if rule.Language == language {
					byName[rule.Name] = rule
				}
			}
			if got, want := slices.Sorted(maps.Keys(byName)), slices.Sorted(maps.Keys(c.hits)); !slices.Equal(got, want) {
				t.Fatalf("patterns = %v, but cases are for %v", got, want)
			}
			for name, lines := range c.hits {
				for _, line := range lines {
					for _, variant := range lineVariants(line, c.statement) {
						if byName[name].Match(variant) == nil {
							t.Errorf("%s/%s missed %q", language, name, variant)
						}
					}
				}
			}
			for _, line := range c.misses {
				for _, variant := range lineVariants(line, c.statement) {
					for name, rule := range byName {
						if rule.Match(variant) != nil {
							t.Errorf("%s/%s matched %q", language, name, variant)
						}
					}
				}
			}
		})
	}
}
