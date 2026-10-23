// Ruleguard file implementing custom linting rules.
//
// Note that when used from golangci-lint (using the gocritic linter configured
// with the ruleguard check), because rule files are not handled by
// golangci-lint itself, changes will not invalidate the golangci-lint cache,
// and you must manually clean to cache (golangci-lint cache clean) for them to
// be considered, as explained here:
// https://www.quasilyte.dev/blog/post/ruleguard/#using-from-the-golangci-lint
//
// Note that this file is ignored from build with a build constraint, but using
// a different than "ignore" to avoid go mod tidy making dsl an indirect
// dependency, as explained here:
// https://github.com/quasilyte/go-ruleguard?tab=readme-ov-file#troubleshooting

//go:build ruleguard

// Package gorules implementing custom linting rules using ruleguard
package gorules

import "github.com/quasilyte/go-ruleguard/dsl"

// Suggest rewriting "log.(Print|Fatal|Panic)(f|ln)?" to
// "fs.(Printf|Fatalf|Panicf)", and do it if running golangci-lint with
// argument --fix. The suggestion wraps a single non-string single argument or
// variadic arguments in fmt.Sprint to be compatible with format string
// argument of fs functions.
//
// Caveats:
//   - After applying the suggestions, imports may have to be fixed manually,
//     removing unused "log", adding "github.com/rclone/rclone/fs" and "fmt",
//     and if there was a variable named "fs" or "fmt" in the scope the name
//     clash must be fixed.
//   - Suggested code is incorrect when within fs package itself, due to the
//     "fs."" prefix. Could handle it using condition
//     ".Where(m.File().PkgPath.Matches(`github.com/rclone/rclone/fs`))"
//     but not sure how to avoid duplicating all checks with and without this
//     condition so haven't bothered yet.
// func useFsLog(m dsl.Matcher) {
// 	m.Match(`log.Print($x)`, `log.Println($x)`).Where(m["x"].Type.Is(`string`)).Suggest(`fs.Log(nil, $x)`)
// 	m.Match(`log.Print($*args)`, `log.Println($*args)`).Suggest(`fs.Log(nil, fmt.Sprint($args))`)
// 	m.Match(`log.Printf($*args)`).Suggest(`fs.Logf(nil, $args)`)
//
// 	m.Match(`log.Fatal($x)`, `log.Fatalln($x)`).Where(m["x"].Type.Is(`string`)).Suggest(`fs.Fatal(nil, $x)`)
// 	m.Match(`log.Fatal($*args)`, `log.Fatalln($*args)`).Suggest(`fs.Fatal(nil, fmt.Sprint($args))`)
// 	m.Match(`log.Fatalf($*args)`).Suggest(`fs.Fatalf(nil, $args)`)
//
// 	m.Match(`log.Panic($x)`, `log.Panicln($x)`).Where(m["x"].Type.Is(`string`)).Suggest(`fs.Panic(nil, $x)`)
// 	m.Match(`log.Panic($*args)`, `log.Panicln($*args)`).Suggest(`fs.Panic(nil, fmt.Sprint($args))`)
// 	m.Match(`log.Panicf($*args)`).Suggest(`fs.Panicf(nil, $args)`)
// }

// Note: fmt.Print* enforcement removed - CLI tools in cmd/ legitimately need it
// To enforce fmt.Print blocking, add rules here with appropriate path filtering

// Reject a message that is one hyphenated token: the project convention is
// Title Case words separated by spaces, and keys, not messages, are kebab-case.
// Good: logutil.Ctx(ctx).Info().Msg("Detected Change")
// Bad:  logutil.Ctx(ctx).Info().Msg("Detected-change")
func logMessageHyphenatedToken(m dsl.Matcher) {
	m.Match(`$_.Msg($msg)`).
		Where(m["msg"].Text.Matches(`^"[A-Za-z0-9]+(-[A-Za-z0-9]+)+"$`) ||
			m["msg"].Text.Matches("^`[A-Za-z0-9]+(-[A-Za-z0-9]+)+`$")).
		Report(`Log message must be Title Case words, not a hyphenated token: $msg`)

	m.Match(`$_.Msgf($fmt, $*_)`).
		Where(m["fmt"].Text.Matches(`^"[A-Za-z0-9%]+(-[A-Za-z0-9%]+)+"$`) ||
			m["fmt"].Text.Matches("^`[A-Za-z0-9%]+(-[A-Za-z0-9%]+)+`$")).
		Report(`Log message must be Title Case words, not a hyphenated token: $fmt`)
}

// Enforce Title Case capitalization in log messages
// Good: logutil.Ctx(ctx).Info().Msg("Processing Video")
// Bad:  logutil.Ctx(ctx).Info().Msg("processing video")
func logMessageCapitalization(m dsl.Matcher) {
	// Match .Msg("...") where message starts with lowercase letter
	m.Match(`$_.Msg($msg)`).
		Where(m["msg"].Text.Matches(`^"[a-z]`)).
		Report(`Log message should start with capital letter: $msg`)

	// Match .Msgf("...", ...) where format string starts with lowercase letter
	m.Match(`$_.Msgf($fmt, $*_)`).
		Where(m["fmt"].Text.Matches(`^"[a-z]`)).
		Report(`Log message should start with capital letter: $fmt`)
}
