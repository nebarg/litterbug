# litterbug

litterbug finds debug statements left in source code, such as `fmt.Println`,
`console.log`, `var_dump` and `dd()`. It can scan files and directories, or run as a Git
pre-commit hook that stops a commit adding them.

**It works with any language, not just the ones it ships with.** It has built-in
patterns for Go, JavaScript and TypeScript, PHP, Python, Ruby, Rust, Java and C#, each
tested against the calls it should find and against code that looks like them but isn't
debugging. Any other language is a few lines of config: the file endings it covers and
the patterns to look for. So you can teach it Elixir's `IO.inspect`, Swift's `dump()` or
anything else yourself, without changing litterbug or waiting for a release. See
[Adding a language](#adding-a-language).

## Install

Each [release](https://github.com/nebarg/litterbug/releases) has litterbug for macOS
and Linux, on `arm64` or `amd64`. On an Apple silicon Mac:

```sh
curl -sL https://github.com/nebarg/litterbug/releases/latest/download/litterbug_darwin_arm64.tar.gz | tar -xz litterbug
mkdir -p ~/.local/bin
mv litterbug ~/.local/bin/
```

For an Intel Mac or Linux, use `darwin_amd64`, `linux_arm64` or `linux_amd64` instead.
`~/.local/bin` needs to be on your `PATH`.

To build it with Go 1.27 or later instead:

```sh
go install github.com/nebarg/litterbug/cmd/litterbug@latest
```

The `--staged` option and the hook need Git.

## Usage

```sh
litterbug                  # scan the current directory
litterbug src main.go      # scan particular directories and files
litterbug --lang go,php    # scan only these languages
litterbug -e '*_test.go'   # skip test files as well
litterbug --staged         # scan only the lines staged for commit
litterbug --count          # only print how many statements were found
```

Each finding shows the file, the line number, the line itself, and the language and
pattern that matched. In a terminal, findings are grouped under their file, with the
matched text in red:

```
internal/api/handler.go
  42  fmt.Println("user", u)  [go/fmt-print]

web/src/cart.ts
  17  console.log(items)  [javascript/console]

2 debug statements in 2 files
```

When the output is piped or captured, as in CI or an IDE, each finding is printed on a
line of its own as `path:line: text`, which editors and scripts can follow:

```
internal/api/handler.go:42: fmt.Println("user", u)  [go/fmt-print]
web/src/cart.ts:17: console.log(items)  [javascript/console]

2 debug statements in 2 files
```

Set the `NO_COLOR` environment variable, or pass `--color=never`, to turn color off.
`--color=always` keeps it when the output is piped or captured, as into `less -R` or a
CI log that shows color.

When nothing matches, litterbug prints `No debug statements found.` Lines longer than
160 characters, such as minified code, are cut down to the part around the match.

The exit status is 0 when nothing is found, 1 when something is, and 2 on an error, so
scripts and CI can tell the cases apart.

`--count` prints only the last line, such as `2 debug statements in 2 files`, and exits
with the same status. It is useful for a quick check of a large tree.

When it walks a directory, litterbug skips files Git ignores, directories whose names
start with a dot, binary files, files over 1 MB, and anything the config or `--exclude`
excludes. A file or directory named on the command line is scanned even if those rules
would skip it.

`--lang` scans only the languages given, including any the config switches off.
`--exclude` adds to the config's exclude patterns, with paths taken from the current
directory. Run `litterbug --help` for every option.

## Adding a language

### Built-in languages

Without a config file, and in one written by `litterbug init-config`, every built-in
language is checked. `litterbug add-language` lists them, and adds any that a config file
doesn't have yet, such as one written with `init-config --lang`, or by an earlier
version of litterbug:

```sh
litterbug add-language                 # list the built-in languages
litterbug add-language python ruby     # add them to this repository's .litterbug.yml
litterbug add-language --global rust   # or to the global config
```

The new languages go after the ones the file has, and the rest of the file, comments
included, is kept as it was. Patterns that also find a program's real output, such as
Python's `print()` or Rust's `println!`, say so in a comment: switch them off with
`enabled: false` if your code prints on purpose.

### Other languages

Any other language can be added by editing the config; there is nothing to build. For
example, to catch leftover `IO.inspect`, `IEx.pry` and `dbg()` calls in Elixir:

1. Write out the default config, to `.litterbug.yml` at the root of the repository,
   unless the repository has one already:

   ```sh
   litterbug init-config
   ```

   Use `litterbug init-config --global` instead to add the language for every
   repository that doesn't have a config of its own.

2. Add the language at the end of the file, under `languages:`, with the file endings it
   covers and a regex for each thing to look for:

   ```yaml
     elixir:
       extensions: [.ex, .exs, .heex]
       patterns:
         inspect:
           regex: '\bIO\.inspect\b'
         pry:
           regex: '\bIEx\.pry\b'
         dbg:
           regex: '(^|[^\w.:])dbg\s*\('
   ```

3. Run litterbug. Elixir files are now checked like any other, both on the command line
   and by the pre-commit hook:

   ```
   lib/cart.ex
     3  require IEx; IEx.pry()  [elixir/pry]
     6  |> IO.inspect(label: "items")  [elixir/inspect]
     9  |> dbg()  [elixir/dbg]

   3 debug statements in 1 file
   ```

`litterbug --lang elixir` scans only the new language, which is handy while you tune
its patterns. Commit `.litterbug.yml` so everyone working in the repository gets the
same checks. [Configuration](#configuration) covers the rest of the format, such as
switching patterns off and excluding files.

## The pre-commit hook

```sh
litterbug install-hook
```

This installs a pre-commit hook in the current repository. The hook runs
`litterbug --staged`, which checks only the lines added in the staged changes, so debug
statements already in a file don't stop a commit. When the hook finds something, Git
stops the commit and shows what was found:

```
cmd/todo/main.go
  102  println("here")  [go/builtin-print]

1 debug statement in 1 file
Remove them, or commit anyway with git commit --no-verify.
```

The hook calls litterbug by its full path, so Git clients that don't share your shell's
`PATH`, such as IDEs, still find it. If you move litterbug, run `install-hook` again to
update the hook. `litterbug uninstall-hook` removes it.

If the repository already has a pre-commit hook from another tool, `install-hook` leaves
it alone. You can run `litterbug --staged` from that hook instead, or replace it with
`litterbug install-hook --force`. With husky, add this line to `.husky/pre-commit`:

```sh
litterbug --staged
```

With lefthook, add it to `lefthook.yml`:

```yaml
pre-commit:
  commands:
    litterbug:
      run: litterbug --staged
```

## Configuration

Without a config file, litterbug uses its built-in defaults, which have every built-in
language: Go, JavaScript and TypeScript (including `.vue` and `.svelte` files), PHP,
Python, Ruby, Rust, Java and C#. [internal/config/languages](internal/config/languages)
has their patterns, in a file for each language. To change them, or
[add a language](#adding-a-language), write the defaults out as a config file, then
edit it:

```sh
litterbug init-config                    # .litterbug.yml at the root of this repository
litterbug init-config --lang python,go   # with only these built-in languages
litterbug init-config --global           # the global config, for repositories without their own
```

litterbug uses one config, never a mix of them:

1. the repository's `.litterbug.yml`, if it has one;
2. otherwise the global config, `~/.config/litterbug/config.yml`, or
   `$XDG_CONFIG_HOME/litterbug/config.yml` when that variable is set;
3. otherwise the built-in defaults.

Because a repository's config is used whole, committing it gives everyone working in the
repository the same checks, whatever their own global config says. Each file or
directory named on the command line, or the current directory without one, uses the
config of the repository it is in, so `litterbug ../other-repo` follows that
repository's config rather than the current one's.

### Format

```yaml
exclude:
  - node_modules
  - vendor

languages:
  go:
    enabled: true
    extensions: [.go]
    patterns:
      fmt-print:
        regex: '\bfmt\.Print(f|ln)?\s*\('
      spew:
        enabled: false
        regex: '\bspew\.\w+\s*\('
```

- Each language lists the file name endings it covers in `extensions`, matched without
  regard to case, and the `patterns` to look for. When several languages cover a file,
  the patterns of all of them apply. For example, a `blade` language for `.blade.php`
  files adds to the `php` patterns.
- Each pattern's `regex` is matched against every line of a file. Patterns use Go's
  regular expression syntax (RE2), which has no lookbehind or backreferences.
- A pattern can also have an `unless` regex, and a line that both regexes match isn't
  reported. Use it where another regex engine would need lookbehind. The default PHP
  `dump-die` pattern uses one so that defining a function or method named `dd` or `dump`
  isn't reported as a call to it:

  ```yaml
  dump-die:
    regex: '(^|[^\w>:$])(dd|ddd|dump)\s*\('
    unless: '\bfunction\s+&?\s*(dd|ddd|dump)\s*\('
  ```

  `unless` applies to the whole line rather than to each match, so a line that defines
  `dump()` and also calls `dd()` isn't reported either.
- `enabled: false` switches a language or a single pattern off without deleting it. When
  `enabled` is left out, it defaults to true.
- `exclude` lists files and directories to skip, written as in a `.gitignore`. A pattern
  without a slash matches a name at any depth, such as `vendor` or `'*_test.go'`. A
  pattern with a slash is a path from the repository root, such as `'dist/**'` or
  `src/Debug/Dumper.php`, and a leading slash makes a single name one too, so `/gen/`
  skips only the `gen` directory at the root. YAML needs patterns that start with `*` to
  be quoted.

A config holds everything that is checked: a language or pattern it leaves out isn't
scanned for. An unknown key is an error, so a typo such as `enable: false` is reported
rather than ignored.

For example, a Go command-line tool whose real output uses `fmt.Println` could run
`litterbug init-config`, then in its `.litterbug.yml` switch off `fmt-print` and skip
test files:

```yaml
exclude:
  - node_modules
  - vendor
  - '*_test.go'

languages:
  go:
    enabled: true
    extensions: [.go]
    patterns:
      fmt-print:
        enabled: false
        regex: '\bfmt\.Print(f|ln)?\s*\('
      # The other Go patterns, and the other languages, as init-config wrote them.
```

### Taking up new defaults

A config file is a copy of the defaults as they were when it was written, so it doesn't
pick up patterns that later versions of litterbug add or fix. The release notes say when
the defaults change. To take them up, run `litterbug init-config --force`, with
`--global` for the global config. It writes the new defaults and keeps the old file as a
`.bak`, so you can copy your changes across. For a repository's config, `git diff` shows
which changes to put back. When a newer version only adds a built-in language,
`litterbug add-language` adds it without touching the rest of the file.

## Limitations

- Patterns match whole lines without knowing which parts are code, comments or strings.
  A commented-out `// console.log(x)` is reported, and so is a debug call written inside
  a string.
- A repository nested inside the one being scanned, such as a submodule, is checked with
  the outer repository's config rather than its own. Scan it on its own, as in
  `litterbug path/to/submodule`, to use its config.
