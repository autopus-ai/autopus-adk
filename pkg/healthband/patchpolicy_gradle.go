package healthband

import "strings"

// The Gradle half of RR-7: an included build (includeBuild in a settings
// file) compiles and runs its sources while Gradle configures a build or an
// IDE syncs it, as buildSrc/ and build-logic/ do, which item 6 denies by
// name. Every includeBuild in a settings file's text counts, inside a
// comment or a string too, so no misreading of comments or string templates
// can hide a call. A call must name its directory with one plain string
// literal, optionally through file(...); anything else, a template, an
// escape, a method reference, or a dynamic method name, leaves the settings
// file unresolved. Included builds that a settings plugin or a script
// applied to the settings file declares are not visible here (RR-8).

const gradleIncludeBuild = "includeBuild"

// gradleIncludedBuilds returns the paths that a settings file's includeBuild
// calls name, relative to its directory, or ok false when a call names its
// build in a way band cannot resolve. kotlin selects the Kotlin DSL, where a
// call needs parentheses; the Groovy DSL also has command calls without them.
func gradleIncludedBuilds(text string, kotlin bool) (paths []string, ok bool) {
	for at := 0; ; {
		i := strings.Index(text[at:], gradleIncludeBuild)
		if i < 0 {
			return paths, true
		}
		start, end := at+i, at+i+len(gradleIncludeBuild)
		at = end
		if start > 0 && gradleIdentChar(text[start-1]) || end < len(text) && gradleIdentChar(text[end]) {
			continue // part of a longer name
		}
		before := strings.TrimRight(text[:start], " \t")
		if strings.HasSuffix(before, "::") || strings.HasSuffix(before, ".&") {
			return nil, false // a method reference may pass anything
		}
		quoted := start > 0 && end < len(text) && strings.ContainsRune(`"'`, rune(text[start-1])) && text[end] == text[start-1]
		rest := strings.TrimLeft(text[end:], " \t")
		var path string
		switch {
		case strings.HasPrefix(rest, "("):
			path, ok = gradleCallArgument(rest[1:], kotlin)
		case kotlin:
			continue // no call in the Kotlin DSL: prose in a comment or a string
		case quoted:
			return nil, false // a Groovy method named by a string
		case rest == "" || strings.ContainsRune("\r\n;,)}", rune(rest[0])):
			continue // a bare name in the Groovy DSL calls nothing
		case rest[0] == '\'' || rest[0] == '"':
			path, ok = gradleCommandArgument(rest)
		default:
			return nil, false // a Groovy command call with another argument
		}
		if !ok {
			return nil, false
		}
		paths = append(paths, path)
	}
}

// gradleCallArgument reads the first argument after a call's "(": a literal
// or file(literal), followed by ")" or ",".
func gradleCallArgument(s string, kotlin bool) (string, bool) {
	s = strings.TrimLeft(s, " \t\r\n")
	wrapped := strings.HasPrefix(s, "file(")
	if wrapped {
		s = strings.TrimLeft(s[len("file("):], " \t\r\n")
	}
	value, rest, ok := gradleLiteral(s, kotlin)
	rest = strings.TrimLeft(rest, " \t\r\n")
	if wrapped && ok {
		ok = strings.HasPrefix(rest, ")")
		rest = strings.TrimLeft(strings.TrimPrefix(rest, ")"), " \t\r\n")
	}
	if !ok || rest == "" || rest[0] != ')' && rest[0] != ',' {
		return "", false
	}
	return value, true
}

// gradleCommandArgument reads a Groovy command call's literal, which must
// end the statement or precede more arguments or a closure.
func gradleCommandArgument(s string) (string, bool) {
	value, rest, ok := gradleLiteral(s, false)
	rest = strings.TrimLeft(rest, " \t")
	if !ok || rest != "" && !strings.ContainsRune("\r\n;,{", rune(rest[0])) && !strings.HasPrefix(rest, "//") {
		return "", false
	}
	return value, true
}

// gradleLiteral reads a string literal that holds no template, escape, or
// newline: a double-quoted or triple-double-quoted string, and in the
// Groovy DSL a single-quoted or triple-single-quoted one.
func gradleLiteral(s string, kotlin bool) (value, rest string, ok bool) {
	for _, quote := range []string{`"""`, "'''", `"`, "'"} {
		if !strings.HasPrefix(s, quote) {
			continue
		}
		if kotlin && quote[0] == '\'' {
			return "", "", false // a character literal
		}
		body := s[len(quote):]
		end := strings.Index(body, quote)
		if end < 0 || strings.ContainsAny(body[:end], "$\\\r\n") {
			return "", "", false
		}
		return body[:end], body[end+len(quote):], true
	}
	return "", "", false
}

// gradleIdentChar reports an ASCII identifier byte of Kotlin and Groovy; any
// other byte ends a name, so an includeBuild next to it counts.
func gradleIdentChar(c byte) bool { return bareKeyChar(c) && c != '-' || c == '$' }
