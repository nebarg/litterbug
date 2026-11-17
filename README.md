# litterbug

litterbug finds debug statements left in source code, such as `fmt.Println`,
`console.log`, `var_dump` and `dd()`. It can scan files and directories, or run as a Git
pre-commit hook that stops a commit adding them.

The languages it knows and the patterns it looks for are kept in a config file, so you
can add a language or a pattern, or switch one off, without changing any code.

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

Set the `NO_COLOR` environment variable to turn color off.

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

Without a config file, litterbug uses its built-in defaults, which cover Go, JavaScript
and TypeScript (including `.vue` and `.svelte` files), and PHP.
[internal/config/default.yml](internal/config/default.yml) lists their patterns. To
change them, write the defaults out as a config file, then edit it:

```sh
litterbug init-config            # .litterbug.yml at the root of this repository
litterbug init-config --global   # the global config, for repositories without their own
```

litterbug uses one config, never a mix of them:

1. the repository's `.litterbug.yml`, if it has one;
2. otherwise the global config, `~/.config/litterbug/config.yml`, or
   `$XDG_CONFIG_HOME/litterbug/config.yml` when that variable is set;
3. otherwise the built-in defaults.

Because a repository's config is used whole, committing it gives everyone working in the
repository the same checks, whatever their own global config says. Each file or
directory litterbug scans uses the config of the repository it is in, so
`litterbug ../other-repo` follows that repository's config rather than the current
one's.

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
- `enabled: false` switches a language or a single pattern off without deleting it. When
  `enabled` is left out, it defaults to true.
- `exclude` lists files and directories to skip, written as in a `.gitignore`. A pattern
  without a slash matches a name at any depth, such as `vendor` or `'*_test.go'`. A
  pattern with a slash is a path from the repository root, such as `'dist/**'` or
  `src/Debug/Dumper.php`. YAML needs patterns that start with `*` to be quoted.

A config holds everything that is checked: a language or pattern it leaves out isn't
scanned for. An unknown key is an error, so a typo such as `enable: false` is reported
rather than ignored.

For example, a Go command-line tool whose real output uses `fmt.Println`, and which also
has some Python scripts, could run `litterbug init-config`, then in its `.litterbug.yml`
switch off `fmt-print`, skip test files, and add a `python` language:

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

  python:
    enabled: true
    extensions: [.py]
    patterns:
      print:
        regex: '(^|[^\w.])print\s*\('
      breakpoint:
        regex: '\bbreakpoint\s*\(|\bpdb\.set_trace\s*\('
```

### Taking up new defaults

A config file is a copy of the defaults as they were when it was written, so it doesn't
pick up patterns that later versions of litterbug add or fix. The release notes say when
the defaults change. To take them up, run `litterbug init-config --force`, with
`--global` for the global config. It writes the new defaults and keeps the old file as a
`.bak`, so you can copy your changes across. For a repository's config, `git diff` shows
which changes to put back.

## Limitations

- Patterns match whole lines without knowing which parts are code, comments or strings.
  A commented-out `// console.log(x)` is reported, and so is a debug call written inside
  a string.
- The PHP `dump-die` pattern also matches the definition of a function or method named
  `dd` or `dump`, as in a debugging helper. Exclude that file, or switch the pattern off
  in the repository's config.
