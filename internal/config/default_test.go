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
	"python": {
		file:      "app.py",
		statement: "x = 1;",
		hits: map[string][]string{
			"breakpoint": {`breakpoint()`, `    breakpoint()`, `if x: breakpoint()`},
			"set-trace": {
				`import pdb; pdb.set_trace()`, `ipdb.set_trace()`, `__import__("pdb").set_trace()`,
				`from pudb import set_trace; set_trace()`, `pdb.Pdb().set_trace()`,
			},
			"icecream": {`ic(x)`, `ic()`, `return ic(x)`},
			"pprint":   {`pprint(x)`, `pprint.pprint(x)`, `pprint.pp(x)`},
			"print":    {`print(x)`, `print("a", file=sys.stderr)`, `if x: print(x)`, `print(a); print(b)`},
		},
		misses: []string{
			`self.print(x)`, `printer(x)`, `sprint(x)`, `blueprint(x)`, `logger.debug(x)`, `def print(self):`,
			`def breakpoint(self):`, `obj.breakpoint()`, `set_breakpoint(x)`, `def set_trace(self):`, `sys.settrace(f)`,
			`logic(x)`, `topic (x)`, `def ic(x):`, `from __future__ import print_function`, `pprint_thing(x)`,
			`self.pprint(x)`, `def pprint(x):`,
		},
	},
	"ruby": {
		file:      "app.rb",
		statement: "x = 1;",
		hits: map[string][]string{
			"binding": {`binding.pry`, `  binding.irb`, `binding.break`, `binding.b`, `binding.pry if x`, `<% binding.pry %>`},
			"byebug":  {`byebug`, `  debugger`, `byebug if x`, `debugger; x += 1`, `debugger(do: "info")`},
			"p": {
				`p x`, `p(x)`, `pp obj`, `pp(obj)`, `ap user`, `p :sym`, `p "msg"`, `p [1, 2]`, `p @x`, `p({a: 1})`,
				`items.each { |i| p i }`,
			},
			"puts": {`puts x`, `puts "msg"`, `puts(x)`, `puts`, `  puts @x`, `puts x if y`},
		},
		misses: []string{
			`binding.local_variable_get(:x)`, `my_binding.pry`, `debugger_enabled = true`, `@debugger = x`, `debugger: true`,
			`attr_reader :debugger`, `debugger = Debugger.new`, `self.debugger`, `if debugger?`, `def debugger`,
			`require "byebug"`, `gem 'byebug'`, `p = 1`, `p.name`, `p + 1`, `return p if done`, `p ? a : b`, `[p, q]`,
			`foo(p)`, `p, q = 1, 2`, `{ |p| p.name }`, `p * 2`, `p ||= x`, `def p(x)`, `cp x`, `apt x`, `obj.p x`,
			`$stdout.puts x`, `STDERR.puts x`, `io.puts(x)`, `def puts(x)`, `puts_list x`, `:puts`, `yield p unless p.nil?`,
			`x = p == q`, `<p class="notice">`, `"<p id=\"x\">hi</p>"`, `%w(-u webrick -p 4567)`, `%w[p q r]`, `h2, p {`,
		},
	},
	"rust": {
		file:      "main.rs",
		statement: "let x = 1;",
		hits: map[string][]string{
			"dbg":   {`dbg!(x)`, `let y = dbg!(x * 2);`, `std::dbg!(x)`, `dbg![x]`, `dbg!()`},
			"print": {`println!("{x}")`, `print!("x")`, `eprintln!("{:?}", x)`, `eprint!("x")`, `std::println!("x")`},
		},
		misses: []string{
			`writeln!(f, "x")`, `format!("{x}")`, `my_dbg!(x)`, `my_println!(x)`, `fn print(&self) {}`, `self.print()`,
			`log::debug!("x")`, `tracing::info!(x)`, `print_stats(x)`, `macro_rules! dbg {`, `eprint_usage()`,
			`println!("cargo:rerun-if-changed=build.rs");`, `println!("cargo::rustc-cfg=x");`,
		},
	},
	"java": {
		file:      "App.java",
		statement: "int x = 1;",
		hits: map[string][]string{
			"stack-trace": {
				`e.printStackTrace();`, `} catch (Exception e) { e.printStackTrace(); }`, `Thread.dumpStack();`,
				`ex.printStackTrace( );`,
			},
			"system-out": {`System.out.println(x);`, `System.err.println("x");`, `System.out.printf("%d%n", x);`, `System.out.print(x);`},
		},
		misses: []string{
			`e.printStackTrace(writer);`, `e.printStackTrace(new PrintWriter(sw));`, `out.println(x);`, `writer.println(x);`,
			`System.out.flush();`, `System.setOut(stream);`, `void printStackTrace() {`, `logger.info("x");`,
			`MySystem.out.println(x);`,
		},
	},
	"csharp": {
		file:      "Program.cs",
		statement: "var x = 1;",
		hits: map[string][]string{
			"debugger":    {`Debugger.Break();`, `System.Diagnostics.Debugger.Break();`, `Debugger.Launch();`, `if (x) Debugger.Break();`},
			"debug-write": {`Debug.WriteLine("x");`, `Debug.Write(x);`, `Debug.Print("x");`, `Debug.Log(x);`, `UnityEngine.Debug.Log(x);`},
			"console": {
				`Console.WriteLine(x);`, `Console.Write(x);`, `Console.Error.WriteLine(e);`, `Console.Out.WriteLine(x);`,
				`@{ Console.WriteLine(x); }`,
			},
		},
		misses: []string{
			`Debug.Assert(x);`, `Debug.LogError("x");`, `Debug.LogWarning(x);`, `Console.ReadLine();`,
			`Console.Out.WriteLineAsync(x);`, `logger.LogDebug(x);`, `console.WriteLine(x);`, `Trace.TraceInformation(x);`,
			`MyDebug.Log(x);`, `if (Debugger.IsAttached) {`,
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
